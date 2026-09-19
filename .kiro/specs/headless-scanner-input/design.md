# Design Document

## Overview

`ScanListener` stops reading `os.Stdin` and instead opens the Scanner_Device — the scanner's evdev character device — reads `input_event` records from it, decodes keycodes into characters, and assembles a Scan_Line that completes on Enter. From the completed barcode value onward, nothing changes: the same `classify` call, the same `modeState`, the same `scan.NewEntryFromLookup`, the same `Queue.CreateScanEntry`.

The terminal used to do two jobs for us, and both move into the process:

1. **Line discipline.** The TTY buffered keystrokes and delivered whole lines. That becomes `lineAssembler` — a pure state machine over Key_Events. This mirrors what `BarcodeInputField.tsx` already does in the browser, which is the same buffer-until-Enter pattern against DOM key events.
2. **Being a visible window into the process.** Nothing replaced this, which is why the current silent failure is invisible. That becomes Scanner_Status on the existing `GET /health` route, plus a mode event on the existing SSE stream.

The device-touching surface is deliberately tiny: one function that opens a path and requests `EVIOCGRAB`. Everything else — record parsing, keycode decoding, shift handling, line assembly, reconnect and backoff — operates on an `io.Reader` or an injected opener and is fully testable without hardware (Requirement 9).

Language: Go. No new module dependencies: `golang.org/x/sys` is already in `go.mod` as an indirect dependency and is promoted to direct.

## Architecture

```
                     cmd/server/main.go
                            │
          ┌─────────────────┼──────────────────────┐
          │ startup         │ startup              │ startup
          ▼                 ▼                      ▼
  events.NewBroadcaster  loadScanListenerConfig  server.NewHandler(
          │                 │                       ..., WithBroadcaster(b),
          │                 │                       WithScannerStatus(listener.Status))
          │                 │                              │
          │                 │                              ▼
          │                 │                     http.ListenAndServe
          │                 │                   (same route set — Req 5.4)
          │                 │                              │
          │                 │                    GET /health reads Status()
          │                 ▼
          │      scanlistener.ScanListener
          │         DevicePath, StockInBarcode, StockOutBarcode,
          │         HeadlessUserID, Open (injectable seam)
          └────────►ModePublisher
                            │
                            │ go listener.Run(ctx)
                            ▼
                  ┌──────────────────────────────┐
                  │ reconnect loop (Req 2)        │
                  │  Open(DevicePath)             │
                  │   ├─ err → log, backoff, retry│
                  │   └─ ok  → readFrom(ctx, dev) │
                  │        device removed → reset │
                  │        buffer, back to top    │
                  └───────────────┬──────────────┘
                                  ▼
                        readKeyEvent(dev)  ── 24-byte input_event records
                                  │ EV_KEY, value==1 only (Req 1.2, 1.3)
                                  ▼
                        lineAssembler.feed(KeyEvent)
                          decodeKey → printable / enter / shift / unmapped
                                  │ complete line on Enter (Req 1.6)
                                  ▼
                        empty? → discard (Req 1.7)
                                  │
                       ┌──────────┴───────────┐
                       │ Control_Barcode?     │
                       ▼ yes                  ▼ no
              mode.set + ModePublisher   LookupService.Lookup
              (Req 5.3)                        │
              no scan entry                    ▼
                                    scan.NewEntryFromLookup
                                    (unchanged shared helper)
                                              │
                                              ▼
                                    Queue.CreateScanEntry
                                    (the Queue returned by NewHandler,
                                     so scan events still broadcast)
```

`scanlistener` still never imports `net` and still registers no route (Requirements 6, 5.4).

## Components and Interfaces

### `KeyEvent` and record parsing — `internal/scanlistener/device.go`

A Linux `input_event` on a 64-bit platform is 24 bytes: a 16-byte `timeval`, then `type` and `code` as `uint16`, then `value` as `int32`. We read the fields we use and skip the timestamp, so there is no dependency on `timeval`'s layout beyond its size.

```go
package scanlistener

// Linux input event types and values we act on. Only EV_KEY presses matter:
// a barcode scanner's release and auto-repeat events carry no information we
// need, and EV_MSC/EV_SYN records interleave with every keystroke.
const (
	evKey      = 0x01
	valuePress = 1

	eventSize = 24 // sizeof(struct input_event) on 64-bit Linux
)

// KeyEvent is one decoded input_event record.
type KeyEvent struct {
	Type  uint16
	Code  uint16
	Value int32
}

// IsKeyPress reports whether this record is a key-press we should decode,
// filtering out non-EV_KEY records and key release/auto-repeat.
func (e KeyEvent) IsKeyPress() bool {
	return e.Type == evKey && e.Value == valuePress
}

// readKeyEvent reads exactly one input_event record. It returns io.EOF only
// when the reader is exhausted at a record boundary; a short read mid-record
// is io.ErrUnexpectedEOF, since a truncated record means we lost framing and
// cannot trust subsequent offsets.
func readKeyEvent(r io.Reader) (KeyEvent, error)
```

`readKeyEvent` uses `binary.NativeEndian` over a fixed 24-byte buffer. Native byte order is correct here because these bytes come from the local kernel, not a wire format.

### Device opening seam — `device_linux.go` / `device_unsupported.go`

The only hardware-touching code, isolated so everything else is testable (Requirement 9.3):

```go
// OpenFunc opens the Scanner_Device at path for reading and reports whether
// Exclusive_Grab was obtained. A false grabbed with a nil error is a usable
// device that other consumers can also read (Requirement 3.2).
type OpenFunc func(path string) (dev io.ReadCloser, grabbed bool, err error)
```

`//go:build linux` provides `openEvdev`: `os.OpenFile(path, os.O_RDONLY, 0)` (Requirement 4.3), then `unix.IoctlSetInt(fd, unix.EVIOCGRAB, 1)`. A grab failure logs and returns `grabbed=false` with a nil error rather than failing the open.

`//go:build !linux` provides an `openEvdev` returning an "unsupported platform" error, so the module still builds and tests on a macOS or Windows development machine. CI runs on Linux, so the real implementation is compiled and vetted on every push.

`ScanListener.Open` defaults to `openEvdev` when nil; tests inject their own.

### Keycode decoding — `internal/scanlistener/keymap.go`

```go
type keyKind int

const (
	keyUnmapped keyKind = iota
	keyPrintable
	keyEnter
	keyShift
)

// decodeKey maps a Linux keycode to the character a US-layout keyboard would
// produce, reporting which category the keycode falls into so the assembler
// can act on Enter and shift without a second lookup.
func decodeKey(code uint16, shift bool) (rune, keyKind)
```

The table covers what HID scanners actually emit: digit row, `KEY_A`–`KEY_Z`, keypad digits `KEY_KP0`–`KEY_KP9` (many scanners are configured to emit these instead of the digit row), `KEY_MINUS`, `KEY_SPACE`, `KEY_ENTER` and `KEY_KPENTER`, and both shift keycodes. Anything else is `keyUnmapped`.

A deliberate scope limit: this is a US-layout map, not a full XKB implementation. A scanner configured for another layout produces wrong characters, and the unmapped-keycode counter in Scanner_Status (Requirement 1.9) is how that gets diagnosed on an appliance with no monitor.

### Line assembly — `internal/scanlistener/assembler.go`

A pure state machine, the direct replacement for the TTY's line discipline:

```go
// lineAssembler accumulates decoded characters into a Scan_Line and reports a
// completed barcode when Enter arrives.
type lineAssembler struct {
	buf      []rune
	shift    bool
	unmapped int
}

// feed processes one Key_Event. It returns ok=true exactly once per Enter,
// with the accumulated line; every other event returns ok=false.
func (a *lineAssembler) feed(e KeyEvent) (line string, ok bool)

// reset discards a partially accumulated Scan_Line. Called on every
// reconnect so characters from two device connections can never be spliced
// into one barcode (Requirement 2.3).
func (a *lineAssembler) reset()
```

Shift state lives in the assembler rather than being tracked globally, so a disconnect that strands a shift key down cannot leak into the next connection — `reset` clears it along with the buffer.

### Reconnect loop — `internal/scanlistener/listener.go`

`Run` changes from a single `bufio.Scanner` pass into an outer reconnect loop around an inner read loop. `Stdin io.Reader` is removed from the struct; `DevicePath string`, `Open OpenFunc`, and the backoff bounds replace it.

```go
// Run opens the Scanner_Device and reads Key_Events from it until ctx is
// cancelled, reopening the device with a capped backoff whenever it is absent
// or disappears. It never returns an error: a missing scanner is an expected
// state on an appliance whose USB enumeration may finish after this process
// starts, so Run keeps retrying and the HTTP server is unaffected either way
// (Requirements 2.1, 2.7).
func (l *ScanListener) Run(ctx context.Context) {
	backoff := l.initialBackoff()
	for ctx.Err() == nil {
		dev, grabbed, err := l.open(l.DevicePath)
		if err != nil {
			l.status.setDisconnected(err)
			if !l.sleep(ctx, backoff) {
				return
			}
			backoff = l.nextBackoff(backoff) // Requirement 2.4
			continue
		}

		l.status.setConnected(grabbed) // Requirement 2.5 resets backoff below
		backoff = l.initialBackoff()
		l.readFrom(ctx, dev)           // returns on device removal or cancellation
		dev.Close()                    // Requirement 3.3
		l.status.setDisconnected(nil)
	}
}
```

`readFrom` owns a `lineAssembler`, calls `reset()` before its first read, and dispatches each completed line into the existing `handleLine` unchanged. `Current_Mode` lives in the `ScanListener` value across reconnects rather than inside `readFrom`, which is what satisfies Requirement 2.6 (a cable interruption does not change direction).

Cancellation while blocked in `read` is handled by closing the device from a goroutine watching `ctx.Done()`; a read on a closed descriptor returns immediately. This avoids putting the device into non-blocking mode and polling.

### Scanner_Status — `internal/scanlistener/status.go`

```go
// Status is the externally observable state of the capture path, surfaced
// through GET /health because an appliance with no monitor has no other way
// to distinguish a dead scanner from an idle one.
type Status struct {
	DevicePath      string     `json:"devicePath"`
	Connected       bool       `json:"connected"`
	Grabbed         bool       `json:"grabbed"`
	Mode            string     `json:"mode"`
	UnmappedKeys    int        `json:"unmappedKeys"`
	LastScanAt      *time.Time `json:"lastScanAt"`
	LastError       string     `json:"lastError,omitempty"`
}

func (l *ScanListener) Status() Status
```

Guarded by a mutex, like `modeState` — whose existing comment already anticipated exactly this use: "the same value could be inspected by future health endpoints."

### Wiring — `internal/server` options and `cmd/server/main.go`

`NewHandler` gains variadic options rather than new positional parameters, so its four existing test call sites (`setup_test.go`, `test_runner_test.go`, `handler_scan_headless_test.go`, `server_properties_test.go`) compile untouched:

```go
type Option func(*config)

// WithScannerStatus supplies the scan listener's status for GET /health. When
// unset, the health response omits the scanner object entirely, which is what
// every existing test sees.
func WithScannerStatus(fn func() scanlistener.Status) Option

// WithBroadcaster supplies an externally owned Broadcaster so the scan
// listener can publish mode changes to the same subscribers the HTTP handlers
// publish to. When unset, NewHandler creates its own, as today.
func WithBroadcaster(b *events.Broadcaster) Option
```

The health handler keeps its existing `"status":"ok"` field verbatim (Requirement 5.2 — `handler_webui_test.go` asserts `bodyContains: "status":"ok"`, and the documented `curl` verification step depends on it) and adds a `scanner` object beside it.

`events.Broadcaster` gains `PublishScannerModeEvent(mode scan.ScanDirection)`, matching the existing `PublishScanEvent` / `PublishInventoryEvent` shape and routing through the same private `publish`.

`main.go` becomes:

```go
broadcaster := events.NewBroadcaster()

listener, listenerOK := loadScanListenerConfig()

opts := []server.Option{server.WithBroadcaster(broadcaster)}
if listenerOK {
	opts = append(opts, server.WithScannerStatus(listener.Status))
}
handler, scanQueue := server.NewHandler(catalog, lookupService, refresher, sqlDB, opts...)

if listenerOK {
	listener.Queue = scanQueue // the broadcasting Queue, per server.go's note
	listener.LookupService = lookupService
	listener.ModePublisher = broadcaster
	go listener.Run(context.Background())
}
```

New environment variable, read with the existing `envOrDefault` helper:

| Env var | Default | Requirement |
|---|---|---|
| `SCANNER_DEVICE` | `/dev/pantry-scanner` | 7.1 |

`STOCK_IN_CONTROL_BARCODE`, `STOCK_OUT_CONTROL_BARCODE`, and `HEADLESS_USER_ID` keep their current names and defaults (Requirement 7.2), and the identical-control-barcode check is unchanged (Requirement 7.3).

## Deployment

### udev rule — `deploy/udev/99-pantry-scanner.rules`

```
SUBSYSTEM=="input", ATTRS{idVendor}=="XXXX", ATTRS{idProduct}=="YYYY", \
  KERNEL=="event*", SYMLINK+="pantry-scanner", GROUP="pantry", MODE="0640"
```

This does two jobs at once. It pins a **stable path**, because `/dev/input/event3` is not stable across reboots or replugs. And it scopes **least privilege to one device** (Requirement 4.2): the alternative one-liner, adding the service user to the `input` group, would grant read access to every input device on the machine — turning Pantry into a keylogger for any keyboard ever attached. The operator reads their scanner's VID/PID off `lsusb` once during setup and fills in the placeholders.

### Compose changes — `deploy/docker-compose.yml`

```yaml
    devices:
      - /dev/pantry-scanner:/dev/pantry-scanner
    group_add:
      - "${SCANNER_GID}"   # numeric: the distroless image has no /etc/group entry
    environment:
      SCANNER_DEVICE: /dev/pantry-scanner
```

`stdin_open: true` and `tty: true` are removed; they existed solely for the `docker attach` workflow (Requirement 8.3). The container keeps running as `nonroot` (uid 65532) — no root, no added capabilities (Requirement 4.1).

### Operational sequencing

Two facts about the existing deployment make ordering matter, and both belong in `deploy/README.md`:

- **Docker refuses to start a container whose `devices:` path does not exist.** So the udev rule must be installed, and the scanner replugged or `udevadm trigger` run, *before* the new Compose file is applied (Requirement 8.5).
- **`pantry-update.service` updates only the image.** It runs `docker compose pull && up -d` against whatever `/opt/pantry/docker-compose.yml` already exists on the Pi; it never refreshes the deployment files themselves. So this feature needs one manual SSH session to install the udev rule and update the Compose file, after which the image update lands on its own (Requirement 8.6).

Because the image can arrive before the Compose file is updated, a container that starts without the device must degrade visibly rather than crash — which is exactly Requirement 2.1 plus Scanner_Status reporting `connected: false`.

The `deploy/README.md` "Barcode Scanner Setup" section is deleted rather than amended. The `docker attach` instructions, the "inputs are consumed only while the container's stdin is attached" limitation, the "Scanner Not Working" troubleshooting steps about stdin, and both warnings that container recreation drops the scanner session all stop being true. Requirement 8.4 is the payoff: the 5-minute update timer no longer breaks scanning.

## Data Models

No schema changes. `ScanListener` still writes ordinary `scan_entries` rows through `scan.Queue.CreateScanEntry`.

`Current_Mode` remains in-memory, still defaulting to `stock_in` at process start, and now additionally survives device reconnects within one process lifetime (Requirement 2.6). Scanner_Status is derived state held in memory and never persisted.

## Error Handling

| Situation | Behavior | Requirement |
|---|---|---|
| `SCANNER_DEVICE` path absent at startup | Log resolved path, status `connected: false`, retry with backoff; HTTP unaffected | 2.1, 7.4 |
| Open fails with a permission error | Same retry path; `LastError` in Scanner_Status names it, so a bad udev rule is diagnosable over HTTP | 2.1, 2.4 |
| `EVIOCGRAB` fails | Log, continue reading, Scanner_Status reports `grabbed: false` | 3.2 |
| Read fails with device removed (`ENODEV`) | Log, close, `assembler.reset()`, back to reconnect loop | 2.2, 2.3 |
| Repeated open failures | Backoff grows to its cap; one log line per attempt, not per read | 2.4 |
| Open succeeds after failures | Log reconnect, reset backoff, preserve `Current_Mode` | 2.5, 2.6 |
| Truncated `input_event` record | Treat as a lost-framing read error: close and reconnect rather than reinterpret subsequent bytes at a wrong offset | 2.2 |
| Unmapped keycode | Discard the event, increment the counter, keep the buffer intact | 1.8, 1.9 |
| Completed line is empty | Discard, no entry created | 1.7 |
| Control_Barcode | Set mode, publish mode event, no entry created | 5.3 |
| `LookupService.Lookup` error | Log, no entry, keep reading | unchanged |
| `Queue.CreateScanEntry` error | Log, keep reading | unchanged |
| Context cancelled | Close device, release grab, return | 2.8, 3.3 |

Nothing in `scanlistener` calls `log.Fatal`; every failure returns control to the reconnect loop.

## Testing Strategy

Per AGENTS.md: table-driven unit tests, `rapid` property tests, and `apitest` cases in `internal/server` for anything observable over HTTP.

**Unit tests**

- `readKeyEvent` (`device_test.go`): a well-formed 24-byte record decodes to the expected `Type`/`Code`/`Value`; a 12-byte reader yields `io.ErrUnexpectedEOF`; an empty reader yields `io.EOF`; two concatenated records decode in order.
- `decodeKey` (`keymap_test.go`): digit row, letters unshifted and shifted, keypad digits, `KEY_ENTER` and `KEY_KPENTER` both returning `keyEnter`, both shift keycodes returning `keyShift`, and an arbitrary unmapped code returning `keyUnmapped`.
- `lineAssembler` (`assembler_test.go`): a digit sequence plus Enter yields the barcode exactly once; a release event (`Value: 0`) for a printable key appends nothing; a non-`EV_KEY` record appends nothing; Enter on an empty buffer yields the empty string, which `handleLine` then discards; shift-down/letter/shift-up yields one uppercase then lowercase; `reset()` mid-line discards the partial buffer and clears stranded shift state.
- Reconnect loop (`listener_test.go`), with an injected `OpenFunc`: an opener failing then succeeding results in a device read without `Run` returning; an opener whose device returns `ENODEV` mid-stream is reopened; backoff grows on consecutive failures and resets after a success; a cancelled context returns from `Run` both while waiting on backoff and while blocked in a read; `Close` is called on every device the opener handed out (no descriptor leak across reconnects).
- Configuration (`cmd/server/main_test.go`): `SCANNER_DEVICE` default and override via `t.Setenv`, matching the existing `TestProductCacheTTL` pattern; the identical-control-barcode case still returns `ok=false`.

**Property tests** (`listener_properties_test.go`, ≥100 iterations each, `pgregory.net/rapid`). Properties 1 and 2 below replace `background-scan-listener`'s Property 1 (empty-line discard) and extend its Property 2 (mode transitions) to drive Key_Event bytes instead of stdin lines. That spec's Properties 3, 4, and 5 cover `scan.NewEntryFromLookup` and attribution, are untouched by this feature, and keep passing as-is.

**`apitest` coverage** (`internal/server`): `GET /health` with no scanner status configured returns `{"status":"ok"}` with no `scanner` field, proving existing behavior is preserved (Requirement 5.2); with a stub status it additionally returns the scanner object. A separate case asserts the route set is unchanged (Requirement 5.4) — `server_properties_test.go`'s existing `/health` fallback property already guards the neighborhood.

**Not covered by automated tests**: `openEvdev`'s two system calls, per Requirement 9.3. Verification there is manual, on the Pi, and the acceptance check is the one thing a spec cannot assert for you — scan a barcode with nothing attached but power and the scanner, and confirm the entry appears in the queue.

## Correctness Properties

### Property 1: Only key-press events of the key type affect capture

For any sequence of Key_Events, the assembled Scan_Line depends only on the subsequence of events whose type is `EV_KEY` and whose value is a key press; inserting any number of key-release events, auto-repeat events, or non-`EV_KEY` records anywhere in the sequence leaves every completed barcode value and the Current_Mode unchanged.

**Validates: Requirements 1.2, 1.3**

### Property 2: Line assembly is exactly delimited by Enter

For any sequence of printable keycodes partitioned by Enter keycodes, the assembler emits exactly one line per Enter, in order, each equal to the concatenation of the decoded characters between the preceding Enter and that one; an empty segment emits the empty string and creates no scan entry; and characters appearing after the final Enter are retained in the buffer rather than emitted.

**Validates: Requirements 1.4, 1.6, 1.7**

### Property 3: Unmapped keycodes are skipped without corrupting the line

For any sequence of printable keycodes with arbitrary unmapped keycodes interleaved, the completed barcode equals the barcode assembled from the printable keycodes alone, and the unmapped-keycode counter equals the number of unmapped keycodes fed.

**Validates: Requirements 1.8, 1.9**

### Property 4: Shift state applies to exactly the keys pressed while held

For any sequence of letter keycodes and shift press/release events, each letter is uppercase if and only if a shift keycode was pressed and not yet released at the moment that letter was fed.

**Validates: Requirement 1.5**

### Property 5: Mode transitions survive reconnects, partial lines do not

For any sequence of device connections, each delivering an arbitrary sequence of Key_Events and then failing with a device-removal error, the Current_Mode after all connections equals the mode set by the last Control_Barcode completed in any connection (or `stock_in` if none), while no barcode value is ever emitted whose characters were fed across two different connections.

**Validates: Requirements 2.3, 2.6**

### Property 6: Reconnect backoff is bounded and reset by success

For any sequence of open outcomes, every wait the ScanListener performs lies between the initial backoff and the configured maximum inclusive, the wait is non-decreasing across consecutive failures, and the first wait after any successful open equals the initial backoff.

**Validates: Requirements 2.4, 2.5**

### Property 7: Device state never affects HTTP availability

For any sequence of open outcomes and read failures, the Server_Process's registered route set and the success of `GET /health` are unchanged, and the `status` field of the health response remains `ok`.

**Validates: Requirements 2.7, 5.2, 5.4**
