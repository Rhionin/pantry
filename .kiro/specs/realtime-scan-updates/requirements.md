# Requirements Document

## Introduction

Today, ScanQueuePage and InventoryPage only reflect scan and inventory changes after the user triggers a local mutation (create, commit, resolve) that calls `loadQueue()` or `loadInventory()` afterward. Changes made through any other path — most notably the headless ScanListener reading from the server's own standard input — are invisible until the user manually reloads the page. This feature adds a single Server-Sent Events endpoint, `GET /api/events`, that streams scan and inventory changes as they happen. Events are published from inside `internal/scan.Queue` and `internal/inventory.Pantry` methods rather than from HTTP handlers, so both browser-triggered and headless-triggered mutations produce live updates through one code path. Each event carries the complete updated `ScanEntry` or `InventoryItem`, which ScanQueuePage and InventoryPage merge directly into their local React state. ShoppingListPage and the suggestion components are out of scope and keep their existing manual-refresh behavior.

## Glossary

- **Server_Process**: The `cmd/server` Go binary that hosts the HTTP handler registered in `internal/server/server.go`.
- **Event_Stream**: The `GET /api/events` HTTP route that streams events to connected browser clients using the server-sent events response format.
- **Event_Broadcaster**: The server-side component that receives events published by Queue and Pantry methods and delivers them to every connection currently open on the Event_Stream.
- **Scan_Event**: A message delivered over the Event_Stream that carries the complete state of one `ScanEntry` (the struct defined in `internal/scan.ScanEntry`).
- **Inventory_Event**: A message delivered over the Event_Stream that carries the complete state of one `InventoryItem` (the struct defined in `internal/inventory.InventoryItem`).
- **Queue**: The existing scan-queue data-access type, `internal/scan.Queue`.
- **Pantry**: The existing inventory data-access type, `internal/inventory.Pantry`.
- **ScanQueuePage**: The existing frontend component at `frontend/src/components/queue/ScanQueuePage.tsx` that displays pending and flagged scan entries.
- **InventoryPage**: The existing frontend component at `frontend/src/components/inventory/InventoryPage.tsx` that displays inventory items.
- **ShoppingListPage**: The existing frontend component at `frontend/src/components/shopping/ShoppingListPage.tsx`.
- **Browser_EventSource**: The browser's built-in `EventSource` API, used by ScanQueuePage and InventoryPage to connect to the Event_Stream.

## Requirements

### Requirement 1: Event stream endpoint

**User Story:** As a user with the pantry web page open, I want the server to push live updates over a single connection, so that my browser learns about scan and inventory changes without polling.

#### Acceptance Criteria

1. THE Server_Process SHALL register a GET /api/events route that responds using the server-sent events response format.
2. WHEN a browser client sends a request to GET /api/events, THE Event_Broadcaster SHALL register that connection to receive Scan_Events and Inventory_Events published after the connection is registered.
3. WHEN a client connected to GET /api/events disconnects, THE Event_Broadcaster SHALL stop sending events to that connection and SHALL release the resources associated with it.
4. IF the Event_Broadcaster fails to write an event to one connected client, THEN THE Event_Broadcaster SHALL remove that client's connection and SHALL continue delivering events to all other connected clients.
5. THE Server_Process SHALL deliver Scan_Events and Inventory_Events exclusively through the GET /api/events route.

### Requirement 2: Scan event publication from the Queue

**User Story:** As a developer, I want scan mutations to publish events from inside the Queue data-access type, so that every code path that changes a scan entry produces a live update through one code path.

#### Acceptance Criteria

1. WHEN Queue.CreateScanEntry successfully creates a scan entry, THE Queue SHALL publish a Scan_Event containing the complete created ScanEntry.
2. WHEN Queue.UpdateScanEntry successfully updates a scan entry, THE Queue SHALL publish a Scan_Event containing the complete updated ScanEntry.
3. WHEN Queue.ResolveFlaggedEntry successfully resolves a flagged scan entry, THE Queue SHALL publish a Scan_Event containing the complete resolved ScanEntry.
4. WHEN Queue.BatchUpdateScanEntries successfully updates a set of scan entries, THE Queue SHALL publish one Scan_Event per updated scan entry, each containing that entry's complete updated state.
5. WHEN Queue.CommitStockIn or Queue.CommitStockOut successfully commits a scan entry, THE Queue SHALL publish a Scan_Event containing the complete committed ScanEntry.
6. IF a Queue method that mutates a scan entry returns an error, THEN THE Queue SHALL NOT publish a Scan_Event for that call.

### Requirement 3: Inventory event publication from the Pantry and Queue

**User Story:** As a developer, I want inventory mutations to publish events regardless of whether they originate from the Pantry or from a scan commit, so that the inventory page reflects stock changes live no matter which code path caused them.

#### Acceptance Criteria

1. WHEN Pantry.AddInstance successfully adds an item instance, THE Pantry SHALL publish an Inventory_Event containing the complete InventoryItem for that instance's item.
2. WHEN Pantry.RemoveInstance successfully removes an item instance, THE Pantry SHALL publish an Inventory_Event containing the complete InventoryItem for that instance's item.
3. WHEN Pantry.UpdateTargetQuantity successfully changes an item's target quantity, THE Pantry SHALL publish an Inventory_Event containing the complete InventoryItem for that item.
4. WHEN Queue.CommitStockIn successfully creates item instances, THE Queue SHALL publish an Inventory_Event containing the complete InventoryItem for the item affected by that commit.
5. WHEN Queue.CommitStockOut successfully removes an item instance, THE Queue SHALL publish an Inventory_Event containing the complete InventoryItem for the item affected by that commit.
6. IF a Pantry or Queue method that mutates inventory state returns an error, THEN THE Pantry or Queue SHALL NOT publish an Inventory_Event for that call.

### Requirement 4: Live scan queue updates in the browser

**User Story:** As a pantry owner viewing the scan queue, I want new and changed scan entries to appear without refreshing, so that I can review scans as they arrive from any source, including headless capture.

#### Acceptance Criteria

1. WHEN ScanQueuePage is displayed, THE ScanQueuePage SHALL open a Browser_EventSource connection to the Event_Stream.
2. WHEN ScanQueuePage receives a Scan_Event for a scan entry with a status of pending or flagged, THE ScanQueuePage SHALL add that entry to its displayed list if no entry with the same ID is currently displayed, or SHALL replace the currently displayed entry with the same ID otherwise.
3. WHEN ScanQueuePage receives a Scan_Event for a scan entry with a status of committed or cancelled, THE ScanQueuePage SHALL remove any displayed entry with the same ID from its displayed list.
4. WHEN ScanQueuePage removes a displayed entry whose ID is present in its current selection, THE ScanQueuePage SHALL remove that ID from its current selection.
5. WHEN ScanQueuePage is removed from the page, THE ScanQueuePage SHALL close its Browser_EventSource connection to the Event_Stream.

### Requirement 5: Live inventory updates in the browser

**User Story:** As a pantry owner viewing inventory, I want stock levels and item details to update without refreshing, so that I see the effect of scans as they are committed.

#### Acceptance Criteria

1. WHEN InventoryPage is displayed, THE InventoryPage SHALL open a Browser_EventSource connection to the Event_Stream.
2. WHEN InventoryPage receives an Inventory_Event for an item, THE InventoryPage SHALL replace the displayed InventoryItem with the same item ID with the received InventoryItem, or SHALL add the received InventoryItem to its displayed list if no item with that ID is currently displayed.
3. WHEN InventoryPage is removed from the page, THE InventoryPage SHALL close its Browser_EventSource connection to the Event_Stream.

### Requirement 6: Connection resilience

**User Story:** As a user, I want the live connection to recover automatically after a network interruption, so that I don't have to manually reconnect.

#### Acceptance Criteria

1. WHILE a Browser_EventSource connection to GET /api/events is interrupted, THE Browser_EventSource SHALL automatically attempt to reconnect to GET /api/events.
2. WHEN a Browser_EventSource connection to GET /api/events reconnects after an interruption, THE Event_Stream SHALL resume delivering Scan_Events and Inventory_Events published after the reconnection, without redelivering events published during the interruption.
3. IF ScanQueuePage or InventoryPage fails to open its Browser_EventSource connection to GET /api/events, THEN THE ScanQueuePage or InventoryPage SHALL continue displaying the data most recently loaded through its existing manual-refresh API calls.

### Requirement 7: Scope boundaries

**User Story:** As a developer, I want the boundaries of this feature made explicit, so that ShoppingListPage and the suggestion components are not modified as part of this change.

#### Acceptance Criteria

1. WHILE displaying the shopping list, THE ShoppingListPage SHALL continue to rely on its existing manual-refresh API calls rather than the Event_Stream.
2. THE Server_Process SHALL publish Scan_Events and Inventory_Events only for changes to scan entries and inventory items, excluding shopping list and consumption-suggestion data.
