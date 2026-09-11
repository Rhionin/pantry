# Requirements Document

## Introduction

Today, capturing a scan requires a browser tab open to a page rendering `BarcodeInputField`, which listens for keystrokes from an HID barcode scanner over the DOM. This feature adds a second capture path that runs inside the Go server process itself: a `ScanListener` that reads completed barcode lines from the Server_Process's own standard input. A USB or Bluetooth HID barcode scanner types into whatever has keyboard focus, just like a physical keyboard, so as long as the terminal window running the server has focus, scanning works with no browser open. Because HID scanners have no direction button, the mode is set by scanning dedicated control barcodes ("STOCK IN" / "STOCK OUT") that persist as the current mode until changed again. Every scan captured this way is queued through the same `Queue.CreateScanEntry` path the existing `ScanCreateHandler` uses, so review still happens through the existing `ScanQueuePage` / `BatchReviewPanel` UI. The existing browser-based `BarcodeInputField` flow is unaffected.

## Glossary

- **ScanListener**: The Go-process component that reads lines from the Server_Process's standard input and turns each into a barcode value.
- **Control_Barcode**: A reserved barcode value (e.g., a printed "STOCK IN" or "STOCK OUT" label) that changes the ScanListener's current mode instead of representing a product.
- **Current_Mode**: The ScanListener's in-memory record of which ScanDirection (stock_in or stock_out) applies to the next product barcode it reads.
- **Product_Barcode**: Any scanned barcode value that is not a Control_Barcode.
- **Queue**: The existing scan-queue data-access type (`internal/scan.Queue`) that both the HTTP scan-create path and the ScanListener write to via `CreateScanEntry`.
- **Headless_User_ID**: The fixed user identifier the ScanListener attaches to scan entries it creates, since there is no browser session to supply one.
- **Server_Process**: The `cmd/server` Go binary, which hosts both the existing HTTP handler and the new ScanListener.

## Requirements

### Requirement 1: Headless HID scan capture via standard input

**User Story:** As a pantry owner, I want the server to read barcode scans typed into its own terminal window by a USB or Bluetooth HID scanner, so that I can stock items in or out without opening a browser.

#### Acceptance Criteria

1. THE Server_Process SHALL start, at Server_Process startup, a ScanListener that reads lines from the Server_Process's own standard input.
2. WHEN the ScanListener reads a complete line from standard input, THE ScanListener SHALL treat the full content of that line as a single barcode value.
3. WHEN standard input reaches end-of-file or returns a read error, THE ScanListener SHALL log that event, SHALL stop reading from standard input, and SHALL NOT attempt to resume reading, while THE Server_Process SHALL continue running and serving existing HTTP routes unaffected.
4. IF a line read from standard input contains zero characters, THEN THE ScanListener SHALL discard that line and SHALL NOT create a scan entry from it.

### Requirement 2: Mode switching via control barcodes

**User Story:** As a pantry owner, I want to scan a printed "STOCK IN" or "STOCK OUT" label to set the direction, so that I can switch modes without a hardware button.

#### Acceptance Criteria

1. THE ScanListener SHALL maintain a Current_Mode value that is one of stock_in or stock_out.
2. WHEN the ScanListener reads a barcode value that exactly matches the configured stock-in Control_Barcode, THE ScanListener SHALL set Current_Mode to stock_in and SHALL NOT create a scan entry for that barcode.
3. WHEN the ScanListener reads a barcode value that exactly matches the configured stock-out Control_Barcode, THE ScanListener SHALL set Current_Mode to stock_out and SHALL NOT create a scan entry for that barcode.
4. WHEN the ScanListener reads a Product_Barcode, THE ScanListener SHALL create a scan entry using the Current_Mode in effect at the time that barcode was read.
5. WHILE no Control_Barcode has been scanned since the Server_Process started, THE ScanListener SHALL use stock_in as the Current_Mode.
6. WHEN the ScanListener sets Current_Mode, THE ScanListener SHALL retain that Current_Mode for all subsequent Product_Barcode scans until another Control_Barcode changes it.
7. IF the configured stock-in Control_Barcode and stock-out Control_Barcode are identical, THEN THE Server_Process SHALL log a configuration error and continue serving existing HTTP routes without a ScanListener.

### Requirement 3: Shared queue integration

**User Story:** As a pantry owner, I want headless scans to go through the same review queue as browser scans, so that I can catch lookup failures and confirm details before they affect inventory.

#### Acceptance Criteria

1. WHEN the ScanListener reads a Product_Barcode, THE ScanListener SHALL create a scan entry through the same product lookup and Queue.CreateScanEntry path used by the existing ScanCreateHandler, setting ProductID to the found product's ID when the lookup succeeds, setting Status to Pending when the lookup succeeds and to Flagged when it does not, and setting UnitCount to 1.
2. THE ScanListener SHALL create every scan entry with a Status of Pending or Flagged and SHALL NOT set that entry's Status to Committed at creation time.
3. THE ScanListener SHALL NOT directly transition any scan entry it creates to Committed status; every such entry SHALL be committed only through the existing POST /api/scans/{id}/commit or POST /api/scans/batch-commit routes, the same routes used to commit browser-created scan entries.
4. WHEN a scan entry created by the ScanListener is listed through GET /api/scans, THE Server_Process SHALL return that entry with the same fields and format as a browser-created scan entry, differing only in the value of the userId field.
5. IF Queue.CreateScanEntry returns an error when the ScanListener attempts to create a scan entry for a Product_Barcode, THEN THE ScanListener SHALL log the error and continue listening for subsequent barcode scans without terminating the Server_Process.

### Requirement 4: Attribution for headless entries

**User Story:** As a developer, I want headless scan entries to carry a defined user identifier, so that they display correctly in the existing review UI and are queryable like any other entry.

#### Acceptance Criteria

1. WHEN the ScanListener creates a scan entry for a Product_Barcode, THE ScanListener SHALL set that entry's user identifier to the configured Headless_User_ID.
2. WHERE no Headless_User_ID is explicitly configured, or WHERE Headless_User_ID is configured as an empty string, THE ScanListener SHALL use the same default user identifier the existing single-user HTTP handlers use.

### Requirement 5: No new network surface

**User Story:** As a developer, I want the headless capture path to avoid introducing new network exposure, so that adding it does not create an unauthenticated remote entry point.

#### Acceptance Criteria

1. THE ScanListener SHALL read scan input exclusively from the Server_Process's own standard input stream and SHALL NOT accept scan input from any network connection.
2. THE ScanListener SHALL NOT open, bind, or listen on any TCP port, UDP port, or Unix domain socket to accept incoming connections.
3. THE Server_Process SHALL NOT register any HTTP route beyond the set of HTTP routes registered by the Server_Process prior to the addition of the ScanListener.

### Requirement 6: Coexistence with browser-based capture

**User Story:** As a pantry owner, I want to keep using the browser scanner input when I have a browser open, so that the existing workflow is not disrupted by this change.

#### Acceptance Criteria

1. THE Server_Process SHALL continue to accept scan entries submitted through the existing POST /api/scans route with the same request fields, validation behavior, and response behavior, regardless of whether the ScanListener is running, stopped, or in an error state.
2. WHILE the ScanListener is running, THE Server_Process SHALL continue to serve the existing browser-based scan creation, listing, review, and commit routes used by BarcodeInputField, ScanQueuePage, and BatchReviewPanel, without requiring any coordination between the ScanListener and those routes.
