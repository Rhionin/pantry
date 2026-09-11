# Design Document

## Overview

`ScanListener` is a new Go component that runs inside `cmd/server` and reads complete lines from the process's own standard input, treating each line as a scanned barcode, then feeds them into the same review queue the browser-based `BarcodeInputField` → `POST /api/scans` path uses. A USB or Bluetooth HID barcode scanner types into whatever has keyboard focus, exactly like a physical keyboard — when that focus is the terminal window running the server, each scan (terminated by the scanner's trailing Enter) arrives as one line on stdin. This is a server-side mirror of `BarcodeInputField.tsx`'s buffer-until-Enter pattern, except the OS terminal driver does the line buffering, so `ScanListener` itself only needs to read whole lines.

Because HID scanners have no direction button, `ScanListener` also owns a small piece of in-memory state, `Current_Mode`, that is changed by scanning one of two reserved "control" barcodes (printed labels, e.g. "STOCK IN" / "STOCK OUT") and applied to every subsequent product barcode until changed again.

Everything stays in the single Go binary against the single SQLite file. No new process, no new network listener, no new HTTP route (Requirement 5). `ScanListener` is purely a producer into `internal/scan.Queue` — the same sink `ScanCreateHandler` writes to — so review, commit, and history all continue to work through the existing `ScanQueuePage` / `BatchReviewPanel` UI unchanged.

Language: Go. Code below is Go, matching existing package conventions.

## Architecture

```
                     cmd/server/main.go
                            │
              ┌─────────────┴─────────────┐
              │ startup                    │ startup
              ▼                            ▼
     server.NewHandler(...)        loadScanListenerConfig()
              │                            │
              ▼                    (control barcodes distinct?)
   http.ListenAndServe                     │
     (existing HTTP routes,                ▼
      unchanged — Req 5.3)         scanlistener.ScanListener
                                      StockInBarcode, StockOutBarcode,
                                      HeadlessUserID
                                            │
                                            │ go listener.Run(ctx)   (background goroutine)
                                            ▼
                                  bufio.Scanner(os.Stdin)
                                            │ one line per scan (Req 1.1, 1.2)
                                            ▼
                                   empty line? → discard (Req 1.4)
                                            │ non-empty
                                  ┌─────────┴─────────────┐
                                  │ matches a Control_Barcode?      │
                                  ▼ yes                              ▼ no (Product_Barcode)
                          update Current_Mode                 product.LookupService.Lookup
                          (no scan entry created)                       │
                                                                        ▼
                                                          scan.NewEntryFromLookup(...)
                                                          (SAME helper ScanCreateHandler
                                                           calls — Req 3.1, 3.4)
                                                                        │
                                                                        ▼
                                                            scan.Queue.CreateScanEntry
                                                          (identical Pending/Flagged rule,
                                                           differs only in UserID — Req 3.4)
```

`ScanListener` never touches `net`, never registers an `http.Handler`, and is not reachable from any HTTP route. The only thing it shares with the HTTP path is the `scan.Queue` and `product.LookupService` it is handed at construction, and the entry-shape helper described below.

### Why a new package

`ScanListener` lives in a new package, `internal/scanlistener`, rather than inside `internal/scan` or `internal/server`:

- It depends on both `internal/scan` (to create entries) and `internal/product` (to look up barcodes) but is not itself a data-access type, so it doesn't belong in `scan` under the AGENTS.md naming rule ("each feature package has exactly one data-access type").
- `internal/server` is exclusively HTTP-route wiring (`server.NewHandler` and the `*Handler` types). `ScanListener` has no `Request`/`Response` shape and no route, so putting it there would blur that boundary and risk future contributors assuming it's reachable over HTTP.

### Operational note: the server's terminal must have focus

Because HID scanners deliver keystrokes to whatever has keyboard focus, the terminal window (or session) running `cmd/server` must be the focused window for scans to reach `ScanListener`. This is a deliberate simplification: no raw device access, no OS-specific input APIs, no elevated permissions — at the cost of requiring the operator to keep that terminal focused while scanning. This constraint should be called out in deployment/operational docs, not solved in code.

## Components and Interfaces

### Mode state and classification

Unchanged from before — pure, dependency-free logic in `internal/scanlistener/mode.go` (already implemented):

```go
type modeState struct {
	mu      sync.Mutex
	current scan.ScanDirection
}

func newModeState() *modeState {
	return &modeState{current: scan.StockIn} // Requirement 2.5
}

func (m *modeState) get() scan.ScanDirection { ... }
func (m *modeState) set(d scan.ScanDirection) { ... }

func classify(barcode, stockInBarcode, stockOutBarcode string) (mode scan.ScanDirection, isControl bool) {
	switch barcode {
	case stockInBarcode:
		return scan.StockIn, true
	case stockOutBarcode:
		return scan.StockOut, true
	default:
		return "", false
	}
}
```

### Shared entry-creation helper (Requirement 3.1, 3.4)

Unchanged from before — `scan.NewEntryFromLookup` in `internal/scan/scan.go` (already implemented), and the `ScanCreateHandler.Handle` refactor to call it (already implemented). Both call sites producing byte-for-byte the same shape of entry is what Requirement 3.4 depends on.

### `ScanListener`

New file `internal/scanlistener/listener.go`:

```go
package scanlistener

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
)

// defaultHeadlessUserID matches the single-user default hardcoded across
// internal/server's handlers (const userID = "user-1").
const defaultHeadlessUserID = "user-1"

// ScanListener reads barcode scans as lines from standard input and feeds
// Product_Barcodes into the shared scan queue, switching Current_Mode on
// Control_Barcodes instead.
type ScanListener struct {
	StockInBarcode  string
	StockOutBarcode string
	HeadlessUserID  string

	Queue interface {
		CreateScanEntry(ctx context.Context, entry scan.ScanEntry) (*scan.ScanEntry, error)
	}
	LookupService interface {
		Lookup(ctx context.Context, barcode, userID string) (product.LookupResult, error)
	}

	// Stdin is the stream ScanListener reads lines from. Defaults to
	// os.Stdin; tests inject their own reader.
	Stdin io.Reader

	// Now supplies the current time. Defaults to time.Now when nil.
	Now func() time.Time
}

// Run reads lines from Stdin until it reaches EOF, hits a read error, or ctx
// is cancelled. It never returns an error: a closed or errored stdin is
// logged and Run returns, so the caller always launches it as a bare
// `go listener.Run(ctx)` with nothing to check (Requirement 1.3).
func (l *ScanListener) Run(ctx context.Context) {
	stdin := l.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	log.Printf("scan listener: reading scans from standard input")

	mode := newModeState()
	scanner := bufio.NewScanner(stdin)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		l.handleLine(ctx, scanner.Text(), mode)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("scan listener: standard input read error, stopping: %v", err)
		return
	}
	log.Printf("scan listener: standard input closed, stopping")
}

func (l *ScanListener) handleLine(ctx context.Context, barcode string, mode *modeState) {
	if barcode == "" {
		return // Requirement 1.4: empty line discarded, no entry created
	}

	if newMode, isControl := classify(barcode, l.StockInBarcode, l.StockOutBarcode); isControl {
		mode.set(newMode)
		return // Requirements 2.2, 2.3: no scan entry for a control barcode
	}

	l.createEntry(ctx, barcode, mode.get())
}

func (l *ScanListener) createEntry(ctx context.Context, barcode string, direction scan.ScanDirection) {
	userID := l.HeadlessUserID
	if userID == "" {
		userID = defaultHeadlessUserID // Requirement 4.2
	}

	lookup, err := l.LookupService.Lookup(ctx, barcode, userID)
	if err != nil {
		log.Printf("scan listener: product lookup for %q failed: %v", barcode, err)
		return
	}

	entry := scan.NewEntryFromLookup(userID, barcode, lookup, &direction, l.now())
	if _, err := l.Queue.CreateScanEntry(ctx, entry); err != nil {
		log.Printf("scan listener: create scan entry for %q failed: %v", barcode, err) // Requirement 3.5
		return
	}
}

func (l *ScanListener) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}
```

Nothing in this package reads a device file, decodes raw bytes, or carries a Linux build tag — `bufio.Scanner` over `os.Stdin` works identically on every OS, and every code path is exercisable in a normal `go test` run by injecting a `strings.Reader` or `io.Pipe` as `Stdin`. There is no more untestable "hardware-only" file in this feature.

### Configuration and lifecycle wiring in `cmd/server/main.go`

New env vars, read with the existing `envOrDefault` helper already in `main.go`:

| Env var | Default | Requirement |
|---|---|---|
| `STOCK_IN_CONTROL_BARCODE` | `STOCK_IN` | 2.2 |
| `STOCK_OUT_CONTROL_BARCODE` | `STOCK_OUT` | 2.3 |
| `HEADLESS_USER_ID` | `user-1` (same default `ScanListener` falls back to internally) | 4.1, 4.2 |

```go
// cmd/server/main.go

func loadScanListenerConfig() (*scanlistener.ScanListener, bool) {
	stockIn := envOrDefault("STOCK_IN_CONTROL_BARCODE", "STOCK_IN")
	stockOut := envOrDefault("STOCK_OUT_CONTROL_BARCODE", "STOCK_OUT")
	if stockIn == stockOut {
		log.Printf("scan listener: STOCK_IN_CONTROL_BARCODE and STOCK_OUT_CONTROL_BARCODE must differ, not starting scan listener")
		return nil, false // Requirement 2.7
	}

	return &scanlistener.ScanListener{
		StockInBarcode:  stockIn,
		StockOutBarcode: stockOut,
		HeadlessUserID:  envOrDefault("HEADLESS_USER_ID", "user-1"),
	}, true
}
```

Wired in `main()`, after `lookupService` is built and before `http.ListenAndServe`:

```go
if listener, ok := loadScanListenerConfig(); ok {
	listener.Queue = scan.NewQueue(sqlDB)
	listener.LookupService = lookupService
	go listener.Run(context.Background())
}

log.Printf("listening on %s", addr)
if err := http.ListenAndServe(addr, handler); err != nil {
	log.Fatalf("listen: %v", err)
}
```

There is no "device configured or not" branch anymore — the listener always starts and reads the process's own stdin; if stdin is closed or not a terminal, `Run` logs that and returns on its own goroutine, and the HTTP server is entirely unaffected either way (Requirement 1.3). The only condition that suppresses starting the listener at all is the stock-in/stock-out barcode collision (Requirement 2.7).

## Data Models

No schema changes. `ScanListener` writes rows through the existing `scan_entries` table via `scan.Queue.CreateScanEntry` — the same table, same columns, same JSON shape (`scan.ScanEntry`) the HTTP path uses. The only new persisted values are ordinary `ScanEntry` rows whose `UserID` happens to be the headless user ID instead of a browser-supplied one.

`Current_Mode` is in-memory only (`modeState`, held inside the `ScanListener` value for the lifetime of the process) and is not persisted — a server restart resets it to `stock_in`, matching Requirement 2.5 ("no Control_Barcode scanned since the Server_Process started").

## No New Network Surface (Requirement 5)

- `ScanListener` reads only from `os.Stdin` (or an injected `io.Reader` in tests). Nothing in the `scanlistener` package imports `net`.
- `server.go`'s route table is untouched by this feature — the same routes registered today (`/health`, the `/api/products*` group, the `/api/scans*` group, `/api/inventory*`, `/api/suggestions/{itemId}`, `/api/items/{itemId}/target-quantity`, the `/api/shopping-list*` group) remain the complete set. `ScanListener` is constructed and started entirely from `cmd/server/main.go`, never from `internal/server`.
- `ScanListener.Run` communicates with the rest of the process only by calling `LookupService.Lookup` and `Queue.CreateScanEntry` — regular Go method calls, not network calls.

## Error Handling

| Situation | `ScanListener` / `main` behavior | Requirement |
|---|---|---|
| Standard input reaches EOF (closed) | `Run` logs and returns; HTTP server keeps running | 1.3 |
| Standard input read error | `Run` logs and returns; no retry; HTTP server keeps running | 1.3 |
| Line is empty | `handleLine` returns immediately, no entry created | 1.4 |
| `STOCK_IN_CONTROL_BARCODE == STOCK_OUT_CONTROL_BARCODE` | `loadScanListenerConfig` logs and returns `ok=false`; no listener started | 2.7 |
| Barcode matches a Control_Barcode | `handleLine` updates `Current_Mode`, returns before any lookup/create call | 2.2, 2.3 |
| `LookupService.Lookup` returns an error | `createEntry` logs and returns; no entry created for that scan; loop continues | (mirrors `ScanCreateHandler`'s `InternalError` path, but headless — nothing to answer) |
| `Queue.CreateScanEntry` returns an error | `createEntry` logs and returns; `Run`'s loop continues reading | 3.5 |

Every error path returns control back to `Run`'s loop, or exits `Run` itself for stdin-level errors — nothing in `ScanListener` ever calls `log.Fatal` or otherwise terminates the process, matching Requirement 3.5's "continue listening... without terminating the Server_Process."

## Testing Strategy

`internal/scanlistener` follows the same table-driven-unit-test + property-test split used elsewhere in the codebase (`internal/scan/scan_test.go` + `scan_properties_test.go`), plus one small addition to the existing `apitest`-based suite in `internal/server`.

**Unit / example tests** (`internal/scanlistener/listener_test.go`), using a `strings.Reader` or `io.Pipe` as `Stdin` and a `fakeLookupService`/`fakeQueue` matching the inline interfaces above:

- Requirement 1.3: `Stdin` reaches EOF → `Run` returns after processing every line already available, without hanging or erroring.
- Requirement 1.4: a blank line in the input produces no created entry.
- Requirement 2.7: `loadScanListenerConfig`-equivalent constructor test with equal stock-in/stock-out barcodes.
- Requirement 5.3: a table/snapshot test on `server.NewHandler`'s registered pattern set (already implicit in existing handler tests; this feature adds no new entry to that set).

**Property tests** (`internal/scanlistener/listener_properties_test.go`), covering the four properties below at ≥100 iterations each using `pgregory.net/rapid` (already a dependency). Because `classify` and `scan.NewEntryFromLookup` are pure functions, most properties need no `Stdin` at all — they call these functions directly with generated inputs. The mode-transition property (Property 2) drives `ScanListener.Run` end-to-end through a generated multi-line `Stdin` reader, so it also exercises the line-reading and dispatch loop, not just `classify` in isolation.

**Integration point with the existing `apitest` suite**: a new case in `internal/server`'s scan-handler tests seeds a `ScanEntry` directly through `scan.NewEntryFromLookup` + `Queue.CreateScanEntry` (standing in for a headless-created entry, with no real terminal input involved) and then issues `GET /api/scans` through the normal `apitest`/`handlerTestCase` flow, asserting the returned JSON has the same shape as a browser-created entry aside from `userId` — directly covering Requirement 3.4 through the real HTTP response path, complementing Property 4's in-process comparison.

Every code path in this feature is exercisable by an automated test — there is no hardware-only or platform-only file left unfaked, since `bufio.Scanner` over an injected `io.Reader` fully stands in for a real terminal.

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system-essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Empty-line discard rule

For any line read from standard input, if the line is empty, the ScanListener discards it, creates no scan entry, and leaves Current_Mode unchanged; if the line is non-empty, its full content is treated as the assembled barcode value passed on to classification.

**Validates: Requirements 1.2, 1.4**

### Property 2: Mode transitions and Control_Barcode vs Product_Barcode classification

For any starting Current_Mode and any sequence of lines where each one is either the configured stock-in Control_Barcode, the configured stock-out Control_Barcode, or any other non-empty value (a Product_Barcode): every occurrence of the stock-in Control_Barcode sets Current_Mode to stock_in and creates no scan entry; every occurrence of the stock-out Control_Barcode sets Current_Mode to stock_out and creates no scan entry; every Product_Barcode creates exactly one scan entry whose direction equals whichever Current_Mode was in effect immediately before that line was processed, without itself changing Current_Mode; and after the full sequence, Current_Mode equals the mode of the last Control_Barcode scanned, or stock_in if the sequence contained no Control_Barcode.

**Validates: Requirements 2.2, 2.3, 2.4, 2.5, 2.6**

### Property 3: Headless entries follow the shared lookup-based creation rule

For any Product_Barcode and any product-lookup result, the scan entry the ScanListener creates has ProductID set to the found product's ID when the lookup result is found and nil when it is not found, has Status set to Pending when found and Flagged when not found, and has UnitCount equal to 1 regardless of the lookup result.

**Validates: Requirements 3.1**

### Property 4: Structural identity with HTTP-created entries

For any barcode, product-lookup result, and direction, the scan entry `scan.NewEntryFromLookup` produces for the ScanListener's headless path is identical, field for field, to the scan entry the same helper produces for the HTTP `ScanCreateHandler` path given the same barcode, lookup result, and direction — except for the UserID field, which differs by construction (Headless_User_ID vs. the HTTP-supplied user ID).

**Validates: Requirements 3.4**

### Property 5: Headless user attribution and default fallback

For any configured Headless_User_ID value, the scan entry the ScanListener creates for a Product_Barcode has UserID equal to that configured value whenever it is non-empty, and has UserID equal to the existing single-user default identifier ("user-1") whenever the configured value is empty or was never set.

**Validates: Requirements 4.1, 4.2**
