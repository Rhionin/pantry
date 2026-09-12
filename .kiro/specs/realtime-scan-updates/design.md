# Design Document

## Overview

Today `ScanQueuePage` and `InventoryPage` only see a mutation after they cause it themselves and call `loadQueue()`/`loadInventory()` afterward. A mutation from any other path — the headless `ScanListener` reading process stdin, or (in principle) a second browser tab — is invisible until the page is manually reloaded.

This feature adds one new server-side component, an `Event_Broadcaster`, and one new route, `GET /api/events`, that streams Server-Sent Events (SSE) to every connected browser. `internal/scan.Queue` and `internal/inventory.Pantry` publish to the broadcaster directly from inside their own mutating methods — not from HTTP handlers — so a mutation made through `POST /api/scans`, through `ScanListener`, or through any future caller all produce the same live update through the same code path. Each event carries the complete `ScanEntry` or `InventoryItem`, so the frontend never has to re-fetch to learn what changed; it merges the payload straight into local React state using the browser's native `EventSource`.

`ShoppingListPage` and the suggestion components are untouched. No new npm package and no new Go module dependency are introduced: SSE is plain `text/event-stream` over the existing `net/http` mux, and the frontend uses `EventSource`, which every supported browser provides natively.

Language: Go (backend) and TypeScript/React (frontend), matching existing package conventions.

## Architecture

```
Browser tab A                                          Browser tab B
     │ EventSource('/api/events')                            │ EventSource('/api/events')
     ▼                                                        ▼
┌─────────────────────────── internal/server ───────────────────────────┐
│  GET /api/events  →  EventsHandler.Handle                             │
│         │ Broadcaster.Subscribe()            (Req 1.2)                │
│         ▼                                                             │
│  for msg := range ch { write "event: ...\ndata: ...\n\n"; Flush() }   │
│         │ write error OR ctx.Done()           (Req 1.3, 1.4)          │
│         ▼                                                             │
│      unsubscribe()                                                    │
└─────────────────────────────────────────────────────────────────────┘
             ▲ PublishScanEvent / PublishInventoryEvent
             │
   internal/events.Broadcaster            (new package; Req 1.1-1.5)
             ▲                        ▲
             │ Broadcaster field      │ Broadcaster field
  internal/scan.Queue          internal/inventory.Pantry
   CreateScanEntry                AddInstance
   UpdateScanEntry                RemoveInstance
   ResolveFlaggedEntry            UpdateTargetQuantity
   BatchUpdateScanEntries              ▲
   CommitStockIn  ───┐                 │ Pantry field (aggregate lookup)
   CommitStockOut ───┴─────────────────┘
     (Req 2.1-2.6, 3.1-3.6)
```

`internal/server/server.go` constructs one `events.Broadcaster`, hands it to both `scan.NewQueue`'s returned `*Queue` and `inventory.NewPantry`'s returned `*Pantry` via an exported field (the same "construct, then assign collaborator fields" pattern `cmd/server/main.go` already uses for `ScanListener.Queue`), and registers the new route. Nothing about the existing `/api/scans*` and `/api/inventory*` handlers changes: they keep returning the mutated resource in the HTTP response exactly as they do today (Requirement 1.5 — events are the *only* thing added, not a replacement for the HTTP response body).

### Why a new package (`internal/events`)

`Broadcaster` is not a data-access type — it holds no database connection and stores nothing durably — so it doesn't fit the AGENTS.md "one data-access type per package" table, and it doesn't belong inside `scan` or `inventory` (either placement would make one of those packages import the other, since `Broadcaster` needs to publish both `scan.ScanEntry` and `inventory.InventoryItem` payloads). A small standalone package avoids that: `events` imports `scan` and `inventory` for their public struct types; neither of those packages imports `events` back. Instead, `scan.Queue` and `inventory.Pantry` each declare their own narrow, locally-defined interface for the collaborator they need (the same pattern `ScanCreateHandler.LookupService` and `ScanCommitHandler.Queue` already use), and `*events.Broadcaster` satisfies both structurally.

## Components and Interfaces

### `internal/events` (new package)

```go
// Package events provides an in-memory publish/subscribe broadcaster that
// delivers Scan_Events and Inventory_Events to every browser connected to
// GET /api/events. It has no persistence: a subscriber only ever receives
// events published after it subscribes (Requirement 1.2), and there is no
// buffering or replay for a client that reconnects after a gap
// (Requirement 6.2 is satisfied by this absence, not by extra bookkeeping).
package events

import (
	"sync"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/scan"
)

// subscriberBuffer bounds how many unread messages a single connection may
// accumulate before it is treated as unable to keep up and dropped, so one
// slow reader can never block delivery to the others (Requirement 1.4).
const subscriberBuffer = 16

// Message is one server-sent event: EventType becomes the SSE "event:" field
// ("scan" or "inventory") and Data is the JSON-encoded ScanEntry or
// InventoryItem that becomes the "data:" field.
type Message struct {
	EventType string
	Data      []byte
}

// Broadcaster fans out published events to every currently-subscribed
// connection. The zero value is not usable; construct with NewBroadcaster.
type Broadcaster struct {
	mu          sync.Mutex
	nextID      int64
	subscribers map[int64]chan Message
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subscribers: make(map[int64]chan Message)}
}

// Subscribe registers a new connection and returns the channel it should
// read published messages from, plus an unsubscribe function the caller
// MUST call exactly once when the connection ends (Requirement 1.3). The
// returned channel receives only messages published after Subscribe returns.
func (b *Broadcaster) Subscribe() (<-chan Message, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++
	ch := make(chan Message, subscriberBuffer)
	b.subscribers[id] = ch

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subscribers, id)
	}
	return ch, unsubscribe
}

// PublishScanEvent delivers entry to every currently-subscribed connection.
func (b *Broadcaster) PublishScanEvent(entry scan.ScanEntry) {
	b.publish("scan", entry)
}

// PublishInventoryEvent delivers item to every currently-subscribed connection.
func (b *Broadcaster) PublishInventoryEvent(item inventory.InventoryItem) {
	b.publish("inventory", item)
}

func (b *Broadcaster) publish(eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("events: marshal %s event: %v", eventType, err)
		return
	}
	msg := Message{EventType: eventType, Data: data}

	b.mu.Lock()
	defer b.mu.Unlock()
	for id, ch := range b.subscribers {
		select {
		case ch <- msg:
		default:
			// Subscriber's buffer is full: it cannot keep up. Drop it here
			// rather than block every other subscriber's delivery
			// (Requirement 1.4's "continue delivering ... to all other
			// connected clients"). EventsHandler treats a dropped/closed
			// subscriber the same way it treats a write error: the browser's
			// native EventSource reconnect (Requirement 6.1) recovers it.
			close(ch)
			delete(b.subscribers, id)
		}
	}
}
```

### `internal/server/handler_events.go` (new file)

SSE is a long-lived streaming response, not a single JSON value, so it does not go through `HandleJSON` — it is a plain `http.HandlerFunc` registered directly on the mux, same as the existing `/health` route.

```go
package server

import (
	"fmt"
	"net/http"

	"github.com/Rhionin/pantry/internal/events"
)

type EventsHandler struct {
	Broadcaster *events.Broadcaster
}

// Handle implements GET /api/events (Requirement 1.1). It blocks for the
// lifetime of the connection, writing one "event: <type>\ndata: <json>\n\n"
// frame per published message until the client disconnects (Requirement 1.3)
// or a write to it fails (Requirement 1.4).
func (h *EventsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	messages, unsubscribe := h.Broadcaster.Subscribe()
	defer unsubscribe()

	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				return // buffer overflow: Broadcaster already dropped us
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.EventType, msg.Data); err != nil {
				return // write failed: stop, defer unsubscribes us
			}
			flusher.Flush()
		case <-r.Context().Done():
			return // client disconnected (Requirement 1.3)
		}
	}
}
```

### `internal/server/server.go` wiring changes

```go
broadcaster := events.NewBroadcaster()

scanQueue := scan.NewQueue(db)
scanQueue.Broadcaster = broadcaster
// ... (existing scan handlers, unchanged)

pantry := inventory.NewPantry(db)
pantry.Broadcaster = broadcaster
scanQueue.Pantry = pantry // lets Queue look up the aggregated InventoryItem after a commit

eventsHandler := &EventsHandler{Broadcaster: broadcaster}
mux.HandleFunc("GET /api/events", eventsHandler.Handle)
```

`scanQueue.Pantry` must be assigned after `pantry` exists, so the inventory handlers block moves up above the scan handlers block, or this assignment moves down after both blocks — either is fine; the actual task will place it after both `scanQueue` and `pantry` exist.

### `internal/scan.Queue` changes

Two new, nil-safe, optional collaborator fields (existing callers of `scan.NewQueue` — every current test — leave them unset and see no behavior change):

```go
type Queue struct {
	db *sql.DB

	// Broadcaster publishes a Scan_Event (and, for commit paths, an
	// Inventory_Event) after each successful mutation. Nil in every existing
	// test and in any caller that doesn't need live updates; publish calls
	// are skipped entirely when nil.
	Broadcaster interface {
		PublishScanEvent(entry ScanEntry)
		PublishInventoryEvent(item inventory.InventoryItem)
	}

	// Pantry supplies the aggregated InventoryItem for a scan commit's
	// affected item, so CommitStockIn/CommitStockOut can publish an
	// Inventory_Event (Requirements 3.4, 3.5) without duplicating
	// inventory's aggregation logic. Only required when Broadcaster is set.
	Pantry interface {
		GetInventoryItem(ctx context.Context, itemID string, now time.Time, warningDays int) (*inventory.InventoryItem, error)
	}
}
```

Each mutating method publishes only after its existing success path, right before its existing `return nil` / `return entry, nil` — never on an error return (Requirement 2.6):

- `CreateScanEntry`: already ends with `return r.GetScanEntry(ctx, entry.ID)`; publish that fetched entry when non-nil and `Broadcaster != nil`, then return it unchanged.
- `UpdateScanEntry`: currently returns only `error`. After `RowsAffected() > 0` confirms the update happened, fetch the entry with `r.GetScanEntry(ctx, id)` and publish it. If that fetch fails, `UpdateScanEntry` returns that error (the row was already updated, but the method's public contract needs the entry to publish, so a fetch failure here is treated the same as any other post-write read failure elsewhere in this file).
- `ResolveFlaggedEntry`: after `tx.Commit()` succeeds, fetch with `r.GetScanEntry(ctx, scanEntryID)` and publish.
- `BatchUpdateScanEntries`: after `tx.Commit()` succeeds, loop over `ids` once more, fetch each with `r.GetScanEntry(ctx, id)`, and publish one `Scan_Event` per ID (Requirement 2.4 — not one event for the whole batch).
- `CommitStockIn` / `CommitStockOut`: after `tx.Commit()` succeeds, fetch the committed entry with `r.GetScanEntry(ctx, scanEntry.ID)` and publish a `Scan_Event` if `r.Broadcaster != nil`; then, if `r.Pantry != nil` and `r.Broadcaster != nil`, fetch the affected item's aggregate with `r.Pantry.GetInventoryItem(ctx, itemID, time.Now(), inventory.DefaultWarningDays)` and publish an `Inventory_Event` (Requirements 3.4, 3.5). `itemID` is already a local variable in both functions today.

None of these fetches happen inside the transaction — they read after `tx.Commit()`, the same way `CreateScanEntry` already reads back through the plain `r.db` connection after its `INSERT`.

### `internal/inventory.Pantry` changes

One new nil-safe field, plus one new method that both the new publish calls and the existing list endpoint use:

```go
type Pantry struct {
	db *sql.DB

	// Broadcaster publishes an Inventory_Event after each successful
	// mutation. Nil in every existing test; publish calls are skipped when nil.
	Broadcaster interface {
		PublishInventoryEvent(item InventoryItem)
	}
}

// DefaultWarningDays is the near-expiry window GetInventoryList and
// GetInventoryItem use when no caller-specific value is required.
const DefaultWarningDays = 7

// GetInventoryItem returns the aggregated InventoryItem for a single item,
// or nil if no such item exists. It computes the same InstanceCount /
// NearExpiryCount / ExpiredCount / NeedsAttention fields GetInventoryList
// computes per item, extracted into aggregateItem so the two methods share
// one implementation.
func (r *Pantry) GetInventoryItem(ctx context.Context, itemID string, now time.Time, warningDays int) (*InventoryItem, error) {
	item, err := r.getItemByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	invItem, err := r.aggregateItem(ctx, *item, now, warningDays)
	if err != nil {
		return nil, err
	}
	return &invItem, nil
}
```

`GetInventoryList`'s per-item loop body (instance fetch + expiry-status tally) is extracted into the shared `aggregateItem(ctx, item Item, now time.Time, warningDays int) (InventoryItem, error)` helper so behavior for a single item and for the full list can never drift apart. `handler_inventory_list.go`'s hardcoded `warningDays := 7` becomes `inventory.DefaultWarningDays` for the same reason.

`AddInstance`, `RemoveInstance`, and `UpdateTargetQuantity` each publish after their existing success path:

- `AddInstance`: after the existing `return r.GetInstance(ctx, instance.ID)` succeeds, fetch `r.GetInventoryItem(ctx, instance.ItemID, time.Now(), DefaultWarningDays)` and publish if non-nil and `Broadcaster != nil`.
- `RemoveInstance`: `existing.ItemID` (already fetched before the removal check) is the affected item; publish after the `UPDATE` succeeds.
- `UpdateTargetQuantity`: `itemID` is already the method's parameter; publish after the `UPDATE` succeeds.

### Frontend: `frontend/src/components/queue/queueUtils.ts` additions

Two small pure functions, tested the same way `sortScansChronologically` already is, so the merge/removal rules are verifiable without rendering React:

```ts
const DISPLAYABLE_SCAN_STATUSES: ScanStatus[] = ['pending', 'flagged'];

// Applies one incoming Scan_Event to the currently displayed list: upserts it
// if its status is still displayable (pending/flagged), or removes any entry
// with the same id if it isn't (committed/cancelled). Requirements 4.2, 4.3.
export const mergeScanEvent = (entries: ScanEntry[], event: ScanEntry): ScanEntry[] => {
  if (!DISPLAYABLE_SCAN_STATUSES.includes(event.status)) {
    return entries.filter((entry) => entry.id !== event.id);
  }
  const index = entries.findIndex((entry) => entry.id === event.id);
  if (index === -1) return [...entries, event];
  return entries.map((entry) => (entry.id === event.id ? event : entry));
};

// Drops any id from a selection that no longer names a displayed entry.
// Requirement 4.4.
export const pruneSelection = (selectedIds: string[], entries: ScanEntry[]): string[] =>
  selectedIds.filter((id) => entries.some((entry) => entry.id === id));
```

### Frontend: `frontend/src/components/inventory/inventoryUtils.ts` (new file)

Mirrors `queueUtils.ts`'s co-located-pure-helper convention for the inventory feature folder:

```ts
import type { InventoryItem } from '../../types';

// Applies one incoming Inventory_Event: replaces the item with a matching
// id, or appends it if no such item is currently displayed. Requirement 5.2.
export const mergeInventoryEvent = (
  items: InventoryItem[],
  event: InventoryItem,
): InventoryItem[] => {
  const index = items.findIndex((item) => item.item.id === event.item.id);
  if (index === -1) return [...items, event];
  return items.map((item) => (item.item.id === event.item.id ? event : item));
};
```

### Frontend: `ScanQueuePage.tsx` changes

A new `useEffect` opens the connection on mount and closes it on unmount (Requirements 4.1, 4.5); its `'scan'` listener merges each event into `entries` and prunes `selectedIds` from the same merged list, in one updater so both stay in sync (Requirement 4.4). No `'error'` listener is added, so a connection failure leaves `entries` exactly as `loadQueue()` last set it (Requirement 6.3) while the native `EventSource` retries in the background (Requirement 6.1).

```tsx
useEffect(() => {
  const eventSource = new EventSource('/api/events');
  eventSource.addEventListener('scan', (message) => {
    const scanEntry = JSON.parse((message as MessageEvent).data) as ScanEntry;
    setEntries((current) => {
      const next = sortScansChronologically(mergeScanEvent(current, scanEntry));
      setSelectedIds((selected) => pruneSelection(selected, next));
      return next;
    });
  });
  return () => eventSource.close();
}, []);
```

### Frontend: `InventoryPage.tsx` changes

Same shape, one listener, no error handling beyond "leave existing state alone" (Requirements 5.1, 5.2, 5.3, 6.3):

```tsx
useEffect(() => {
  const eventSource = new EventSource('/api/events');
  eventSource.addEventListener('inventory', (message) => {
    const inventoryItem = JSON.parse((message as MessageEvent).data) as InventoryItem;
    setInventoryItems((current) => mergeInventoryEvent(current, inventoryItem));
  });
  return () => eventSource.close();
}, []);
```

`ShoppingListPage.tsx` and the `suggestions` components receive no changes (Requirement 7.1); nothing in `internal/shopping` or `internal/suggestion` gains a `Broadcaster` field (Requirement 7.2).

## Data Models

No schema changes and no new persisted fields. `events.Message` and the SSE wire frames it produces are transient: `EventType` becomes the SSE `event:` line, `Data` is the same JSON encoding of `scan.ScanEntry` / `inventory.InventoryItem` the HTTP endpoints already return, so the frontend's existing `ScanEntry` / `InventoryItem` TypeScript types (`frontend/src/types/index.ts`) apply unchanged to event payloads — no new frontend type is needed.

## Error Handling

| Situation | Behavior | Requirement |
|---|---|---|
| A `scan.Queue` or `inventory.Pantry` mutation returns an error | No publish call is made; the method's existing error return is unchanged | 2.6, 3.6 |
| `Broadcaster` field is nil (no SSE wiring, e.g. every existing unit/property test) | Publish call is skipped entirely; mutation behaves exactly as it does today | — |
| A subscriber's channel buffer fills up (slow reader) | `Broadcaster.publish` closes and drops that subscriber; delivery to every other subscriber continues in the same call | 1.4 |
| Writing an SSE frame to a connection fails | `EventsHandler.Handle` returns, its `defer unsubscribe()` runs, no further messages are attempted for that connection | 1.3, 1.4 |
| Client disconnects (context cancelled) | Same as a write failure: `Handle` returns, `unsubscribe()` runs | 1.3 |
| `http.ResponseWriter` doesn't support `http.Flusher` | `Handle` responds `500` immediately without subscribing | — (defensive; not reachable with `net/http`'s standard server) |
| `EventSource` fails to connect or the connection drops | Handled entirely by the browser's native reconnect; `ScanQueuePage`/`InventoryPage` add no `'error'` listener, so displayed state stays whatever the last manual-refresh load produced | 6.1, 6.3 |

## Testing Strategy

**`internal/events`** (new `broadcaster_test.go` + `broadcaster_properties_test.go`, following the `scan`/`inventory` package's unit-plus-property split): unit tests construct a `Broadcaster`, subscribe, publish, and assert delivery/non-delivery for the specific ordering and buffer-overflow scenarios called out in Error Handling. Property tests use `pgregory.net/rapid` (already a dependency) to cover Properties 1-3 below with generated subscriber counts, disconnect subsets, and publish sequences.

**`internal/scan` and `internal/inventory`**: the publish behavior added to `Queue`/`Pantry` methods is an internal collaboration contract (a fake `Broadcaster`/`Pantry` recording calls), not something visible in an HTTP response body — per AGENTS.md, this is exactly the case where a unit/property test is preferable to an API test. New test doubles `fakeBroadcaster` (records every `PublishScanEvent`/`PublishInventoryEvent` call) are added alongside the existing `newTestQueue`/`newTestPantry` helpers, assigned to the `Broadcaster` (and, for `scan_test.go`, `Pantry`) field before exercising each method. Properties 4-8 below extend `scan_properties_test.go` and `aggregate_properties_test.go` with this fake in place, reusing the same `rapid` generators the existing properties in those files already use (e.g. `TestProperty4_BatchUpdateApplies`'s batch-selection generator, for the new batch-publish property).

**`internal/server`**: `GET /api/events` is a long-lived stream, which the `handlerTestCase`/`exchanges()` table framework isn't shaped for (it asserts on a single completed response). One dedicated integration test in a new `handler_events_test.go` follows the same `httptest.NewServer` + raw `http.Client` pattern `handler_scan_headless_test.go`'s `getScanEntry` helper already uses elsewhere in this package: start a real server, open a streaming GET to `/api/events` in a goroutine, trigger a mutation through a normal `POST /api/scans` call, and assert the expected `event: scan` frame arrives on the stream. This is the one case in this feature where an integration test, not the declarative `handlerTestCase` table, is the right tool, precisely because SSE isn't a single request/response exchange.

**Frontend**: `queueUtils.test.ts` and a new `inventoryUtils.test.ts` gain example/property-style tests for `mergeScanEvent`, `pruneSelection`, and `mergeInventoryEvent` directly (Properties 9-11), with no rendering required. `ScanQueuePage.test.tsx` and `InventoryPage.test.tsx` gain example tests for mount/unmount `EventSource` wiring and the "connection fails, existing data stays" case (Requirements 4.1, 4.5, 5.1, 5.3, 6.3), using a small fake `EventSource` class installed via `vi.stubGlobal('EventSource', FakeEventSource)` that records constructor calls, lets a test dispatch a named event or an `'error'` event, and records `close()` calls — the same `vi.stubGlobal` technique these test files already use for `fetch`.

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system-essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Subscribers only receive events published after they subscribe

For any sequence of `PublishScanEvent`/`PublishInventoryEvent` calls split into a "before" group and an "after" group by when a new `Subscribe()` call occurs between them, the channel returned by that `Subscribe()` call SHALL receive exactly the messages in the "after" group, in publish order, and SHALL receive none of the messages in the "before" group.

**Validates: Requirements 1.2**

### Property 2: Unsubscribing removes exactly that subscriber without affecting the rest

For any number of concurrently subscribed connections and any subset of them whose `unsubscribe` function is called before a subsequent publish, that publish SHALL deliver the message to every subscriber not in the unsubscribed subset and SHALL deliver it to none of the subscribers in the unsubscribed subset, and the broadcaster's subscriber registry SHALL no longer contain any entry for an unsubscribed connection.

**Validates: Requirements 1.3**

### Property 3: A subscriber that cannot keep up is dropped without blocking delivery to others

For any set of subscribed connections where an arbitrary non-empty subset never drain their channel until it fills, publishing enough events to overflow those channels SHALL cause every subscriber outside that subset to receive every published event, regardless of how many events were needed to overflow the non-draining subset's buffers.

**Validates: Requirements 1.4**

### Property 4: Every successful scan-mutating operation publishes exactly one matching Scan_Event

For any call to `CreateScanEntry`, `UpdateScanEntry`, `ResolveFlaggedEntry`, `CommitStockIn`, or `CommitStockOut` that returns without an error, exactly one `PublishScanEvent` call SHALL occur whose `ScanEntry` argument deep-equals the entry's complete state immediately after that call, as read back through `GetScanEntry`.

**Validates: Requirements 2.1, 2.2, 2.3, 2.5**

### Property 5: Batch update publishes one Scan_Event per updated entry

For any non-empty subset of scan entries passed to `BatchUpdateScanEntries` that returns without an error, exactly one `PublishScanEvent` call SHALL occur per entry in that subset, each carrying that entry's complete post-update state, and no `PublishScanEvent` call SHALL occur for any entry outside the subset.

**Validates: Requirements 2.4**

### Property 6: A failed scan-mutating call publishes no Scan_Event

For any call to `CreateScanEntry`, `UpdateScanEntry`, `ResolveFlaggedEntry`, `BatchUpdateScanEntries`, `CommitStockIn`, or `CommitStockOut` driven into one of its documented error conditions, the call SHALL return an error and zero `PublishScanEvent` calls SHALL occur as a result of that call.

**Validates: Requirements 2.6**

### Property 7: Every successful inventory-affecting operation publishes exactly one matching Inventory_Event

For any call to `Pantry.AddInstance`, `Pantry.RemoveInstance`, `Pantry.UpdateTargetQuantity`, `Queue.CommitStockIn`, or `Queue.CommitStockOut` that returns without an error, exactly one `PublishInventoryEvent` call SHALL occur whose `InventoryItem` argument deep-equals the affected item's complete aggregated state immediately after that call, as read back through `GetInventoryItem`.

**Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5**

### Property 8: A failed inventory-affecting call publishes no Inventory_Event

For any call to `Pantry.RemoveInstance`, `Pantry.UpdateTargetQuantity`, `Queue.CommitStockIn`, or `Queue.CommitStockOut` driven into one of its documented error conditions, the call SHALL return an error and zero `PublishInventoryEvent` calls SHALL occur as a result of that call.

**Validates: Requirements 3.6**

### Property 9: Merging a Scan_Event upserts displayable entries and removes non-displayable ones

For any list of displayed scan entries and any incoming `Scan_Event`, `mergeScanEvent` SHALL produce a list containing an entry with the event's id, equal to the event, exactly once when the event's status is pending or flagged, and SHALL produce a list containing no entry with the event's id when the event's status is committed or cancelled; in both cases every other entry already in the list SHALL remain unchanged and in the same relative order.

**Validates: Requirements 4.2, 4.3**

### Property 10: Removing an entry from the displayed list prunes it from the selection

For any list of selected entry ids and any list of currently displayed entries, `pruneSelection` SHALL produce a list containing exactly the ids from the input that name an entry present in the displayed list, in the same relative order, and no others.

**Validates: Requirements 4.4**

### Property 11: Merging an Inventory_Event upserts the matching item

For any list of displayed inventory items and any incoming `Inventory_Event`, `mergeInventoryEvent` SHALL produce a list containing an item with the event's item id, equal to the event, exactly once, and every other item already in the list SHALL remain unchanged and in the same relative order.

**Validates: Requirements 5.2**
