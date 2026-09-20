# Implementation Plan

## Overview

Build the capture path bottom-up, in the order that keeps everything testable without
hardware: record parsing, then keycode decoding, then line assembly — three pure,
dependency-free units — then the device-opening seam, then the reconnect loop that
composes them, then the status surface, then wiring, then deployment.

The three pure units land first because they carry all the logic that the TTY used to
provide for free, and because Requirement 9 makes their testability a requirement rather
than a nicety. `openEvdev`'s two system calls are the only code in this plan not
exercisable by `go test`, and they are reached through an injectable `OpenFunc` so the
reconnect loop above them is fully faked in tests.

Everything downstream of a completed barcode line is untouched: `classify`, `modeState`,
`handleLine`, `scan.NewEntryFromLookup`, and `Queue.CreateScanEntry` keep their current
behavior, and `background-scan-listener`'s Properties 3, 4, and 5 keep passing unmodified.

`ScanListener.Stdin` is **retained**. Today's `bufio.Scanner` loop moves verbatim into
`runStdin` and stays selectable via `SCAN_INPUT=stdin` for local development, so a
developer with no scanner and no evdev subsystem can still exercise control barcodes and
mode switching. The existing `listener_test.go` cases and the stdin-driven property tests
from `background-scan-listener` are therefore amended only in their fixture setup, not
rewritten — if a task below appears to require changing their assertions, that is a
signal the stdin path's behavior was altered, which this feature does not do.

Language: Go. Verification follows AGENTS.md: table-driven unit tests, `rapid` property
tests (`pgregory.net/rapid`, already a dependency), and the `apitest` framework in
`internal/server` via `handlerTestCase` / `runHandlerTests`.

`server.NewHandler` gains variadic options rather than positional parameters, so its four
existing test call sites compile untouched.

## Tasks

- [x] 1. Implement `input_event` record parsing
  - [x] 1.1 Create `internal/scanlistener/device.go` with the `evKey`, `valuePress`,
    and `eventSize` constants, the `KeyEvent` struct (`Type uint16`, `Code uint16`,
    `Value int32`), `KeyEvent.IsKeyPress()`, and `readKeyEvent(r io.Reader)
    (KeyEvent, error)`
    - `readKeyEvent` reads exactly `eventSize` (24) bytes and decodes `Type`,
      `Code`, and `Value` with `binary.NativeEndian`, skipping the leading
      16-byte `timeval`
    - Return `io.EOF` only when the reader is exhausted at a record boundary;
      return `io.ErrUnexpectedEOF` for a short read mid-record, since lost
      framing makes every subsequent offset untrustworthy
    - _Requirements: 1.2, 1.3_

  - [x] 1.2 Write unit tests for `readKeyEvent` and `IsKeyPress` in
    `internal/scanlistener/device_test.go`
    - A well-formed 24-byte record decodes to the expected field values
    - Two concatenated records decode in order from one reader
    - A 12-byte reader yields `io.ErrUnexpectedEOF`; an empty reader yields `io.EOF`
    - `IsKeyPress` is true only for `Type: evKey` with `Value: 1`; false for
      `Value: 0` (release), `Value: 2` (auto-repeat), and any non-`evKey` type
    - _Requirements: 1.2, 1.3_

- [x] 2. Implement keycode decoding
  - [x] 2.1 Create `internal/scanlistener/keymap.go` with the `keyKind` enum
    (`keyUnmapped`, `keyPrintable`, `keyEnter`, `keyShift`) and `decodeKey(code
    uint16, shift bool) (rune, keyKind)`
    - Cover the digit row, `KEY_A`–`KEY_Z`, keypad digits `KEY_KP0`–`KEY_KP9`,
      `KEY_MINUS`, `KEY_SPACE`, `KEY_ENTER` and `KEY_KPENTER` as `keyEnter`, and
      both shift keycodes as `keyShift`; everything else is `keyUnmapped`
    - Document that this is a US-layout map by design, and that Scanner_Status's
      unmapped-keycode counter is how a differently configured scanner is
      diagnosed on a machine with no monitor
    - _Requirements: 1.4, 1.5, 1.8_

  - [x] 2.2 Write unit tests for `decodeKey` in
    `internal/scanlistener/keymap_test.go`
    - Digit row and keypad digits both decode to the same digit characters
    - Letters decode lowercase with `shift=false` and uppercase with `shift=true`
    - `KEY_ENTER` and `KEY_KPENTER` both return `keyEnter`; both shift keycodes
      return `keyShift`; an arbitrary high keycode returns `keyUnmapped`
    - _Requirements: 1.4, 1.5, 1.8_

- [x] 3. Implement line assembly
  - [x] 3.1 Create `internal/scanlistener/assembler.go` with `lineAssembler`
    (`buf []rune`, `shift bool`, `unmapped int`), `feed(e KeyEvent) (line string, ok
    bool)`, `reset()`, and an accessor for the unmapped count
    - `feed` returns `ok=true` exactly once per Enter key-press, carrying the
      accumulated line, and clears the buffer at that point
    - `feed` ignores every event for which `IsKeyPress()` is false
    - A `keyShift` press sets shift state and a shift release clears it, so
      `feed` must observe releases for shift keycodes specifically while
      ignoring releases for every other key
    - A `keyUnmapped` press increments the counter and leaves the buffer intact
    - `reset` clears the buffer *and* the shift state, so a disconnect that
      strands a shift key down cannot leak into the next connection
    - _Requirements: 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 2.3_

  - [x] 3.2 Write unit tests for `lineAssembler` in
    `internal/scanlistener/assembler_test.go`
    - A digit sequence followed by Enter yields that barcode exactly once, and a
      second `feed` of the same Enter does not re-emit it
    - A release event for a printable key appends nothing; a non-`EV_KEY` record
      appends nothing
    - Enter on an empty buffer yields `("", true)` — the empty value that
      `handleLine` then discards
    - shift-down, letter, shift-up, letter yields one uppercase then one lowercase
    - An unmapped keycode between two digits produces the two digits
      concatenated and an unmapped count of 1
    - `reset()` mid-line discards the partial buffer, and a subsequent line is
      assembled from scratch; `reset()` while shift is held clears the shift state
    - _Requirements: 1.5, 1.6, 1.7, 1.8, 1.9, 2.3_

- [x] 4. Checkpoint - parsing, decoding, and assembly compile and their tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 5. Implement the device-opening seam
  - [x] 5.1 Add the `OpenFunc` type to `internal/scanlistener/device.go`
    - `type OpenFunc func(path string) (dev io.ReadCloser, grabbed bool, err error)`,
      documenting that `grabbed=false` with a nil error is a usable device that
      other consumers can also read
    - _Requirements: 3.1, 3.2, 9.3_

  - [x] 5.2 Create `internal/scanlistener/device_linux.go` with `//go:build linux`
    implementing `openEvdev`
    - `os.OpenFile(path, os.O_RDONLY, 0)` — read-only, per Requirement 4.3
    - `unix.IoctlSetInt(fd, unix.EVIOCGRAB, 1)`; on failure, log and return
      `grabbed=false` with a nil error rather than failing the open
    - Promote `golang.org/x/sys` from an indirect to a direct dependency in
      `go.mod`
    - _Requirements: 3.1, 3.2, 4.1, 4.3_

  - [x] 5.3 Create `internal/scanlistener/device_unsupported.go` with `//go:build
    !linux` implementing `openEvdev` as an "unsupported platform" error
    - Keeps the module building and testing on a macOS or Windows development
      machine; CI runs on Linux, so the real implementation is still compiled
      and vetted on every push
    - _Requirements: 9.3_

- [x] 6. Rework `ScanListener` into a reconnecting device reader
  - [x] 6.1 Create `internal/scanlistener/status.go` with the `Status` struct
    (`DevicePath`, `Connected`, `Grabbed`, `Mode`, `UnmappedKeys`, `LastScanAt`,
    `LastError`), a mutex-guarded holder, and `(*ScanListener).Status() Status`
    - JSON tags as specified in the design, with `LastError` omitted when empty
    - _Requirements: 1.9, 5.1_

  - [x] 6.2 Add `runDevice` and `readFrom` to `internal/scanlistener/listener.go`
    - **Keep** the `Stdin io.Reader` field; add `Source Source`, `DevicePath
      string`, `Open OpenFunc`, and the initial/maximum backoff fields
    - `runDevice` loops while `ctx.Err() == nil`: open, and on error record the status,
      wait the backoff, grow it toward the cap, and retry; on success record the
      status, reset the backoff, run `readFrom`, then close the device
    - `readFrom` owns a `lineAssembler`, calls `reset()` before its first read,
      reads via `readKeyEvent`, and dispatches each completed line into the
      existing `handleLine` unchanged
    - Hold `modeState` on the `ScanListener` value rather than inside `readFrom`,
      so `Current_Mode` survives reconnects
    - Handle cancellation while blocked in a read by closing the device from a
      goroutine watching `ctx.Done()`
    - Treat a truncated record as a lost-framing read error: close and reconnect
    - _Requirements: 1.1, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 3.3, 4.3_

  - [x] 6.3 Add a mode-change publisher hook to `ScanListener`
    - Add a `ModePublisher` field with an inline single-method interface
      (`PublishScannerModeEvent(mode scan.ScanDirection)`), called from
      `handleLine` when a Control_Barcode changes the mode; a nil publisher is a
      no-op so tests and a listener-less server need no stub
    - _Requirements: 5.3_

  - [x] 6.4 Extend `internal/scanlistener/listener_test.go` with device-path cases
    - Keep every existing stdin case, adding only `Source: SourceStdin` to its
      fixture; their assertions must not change (Requirement 10.7)
    - Add cases driven by an injected `OpenFunc` over a `bytes.Reader` of
      synthetic `input_event` records, reusing the existing
      `fakeQueue`/`fakeLookupService`
    - An opener that fails once then succeeds reads from the device without
      `Run` returning
    - An opener whose device returns a device-removal error mid-stream is
      reopened, and a barcode split across the failure boundary is never emitted
    - Backoff grows across consecutive failures and resets to its initial value
      after a success
    - A cancelled context returns from `Run` both while waiting on backoff and
      while blocked in a read
    - Every device the opener hands out is closed exactly once — no descriptor
      leak across reconnects
    - A Control_Barcode invokes the `ModePublisher`; a Product_Barcode does not
    - `Status()` reports `connected` true while reading and false after a
      removal, and carries the open error in `LastError`
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.8, 3.3, 5.1, 5.3_

  - [x] 6.5 Create `internal/scanlistener/source.go` with the `Source` type, the
    `SourceDevice` and `SourceStdin` constants, and `ParseSource(raw string)
    (Source, bool)`
    - An unset, empty, or unrecognized value yields `SourceDevice`; an
      unrecognized value additionally yields `ok=false` so the caller can log it
    - Document why the default is the device rather than stdin: a typo or a stale
      config must not silently select the source that cannot work under a
      service manager
    - _Requirements: 7.5, 7.6, 7.7, 10.6_

  - [x] 6.6 Move today's stdin loop into `runStdin` and add the TTY mismatch guard
    - `runStdin` is the current `Run` body verbatim: `bufio.Scanner` over
      `l.Stdin` defaulting to `os.Stdin`, stopping on EOF or a read error with no
      retry, dispatching each line into `handleLine`
    - `Run` becomes a `switch l.Source` dispatching to `runStdin` or `runDevice`
    - At `runStdin` entry, check whether standard input is a terminal using
      `github.com/mattn/go-isatty` — promoted from indirect to direct in `go.mod`
      — and when it is not, log a warning naming the combination and record it in
      the status `LastError`, then proceed
    - Do not use `os.Stdin.Stat()` against `os.ModeCharDevice` for this check:
      `/dev/null` is a character device and would pass it, which is precisely the
      case the guard exists to catch
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5, 10.6_

  - [x] 6.7 Write unit tests for source selection and the stdin guard
    - `source_test.go`: `ParseSource` table covering `"device"`, `"stdin"`, `""`,
      and an arbitrary other string
    - `listener_test.go`: with `Source: SourceStdin` and an injected `Stdin`,
      `Run` reads lines and never invokes the `OpenFunc`; with `Source:
      SourceDevice`, `Run` invokes the `OpenFunc` and never reads `Stdin`
    - `Status()` reports the active source under each configuration
    - _Requirements: 7.5, 7.6, 7.7, 10.1, 10.2, 10.6_

- [x] 7. Checkpoint - the listener compiles against a faked device and its tests pass
  - Run `go test -race ./internal/scanlistener/...` for the reconnect loop's
    context-cancellation goroutine and the status mutex
  - Ensure all tests pass, ask the user if questions arise.

- [x] 8. Surface Scanner_Status and mode events through existing routes
  - [x] 8.1 Add `PublishScannerModeEvent(mode scan.ScanDirection)` to
    `internal/events/broadcaster.go`
    - Route through the existing private `publish`, matching the shape of
      `PublishScanEvent` and `PublishInventoryEvent`
    - _Requirements: 5.3_

  - [x] 8.2 Add variadic options to `server.NewHandler` in
    `internal/server/server.go`
    - `type Option func(*config)`, `WithScannerStatus(fn func()
      scanlistener.Status)`, and `WithBroadcaster(b *events.Broadcaster)`
    - `WithBroadcaster` unset preserves today's behavior of constructing a
      Broadcaster internally; the four existing test call sites must compile
      unchanged
    - _Requirements: 5.1, 5.3, 5.4_

  - [x] 8.3 Extend the `GET /health` handler with the scanner object
    - Keep the existing `"status":"ok"` field byte-for-byte; add a `scanner`
      object beside it built from the configured status function
    - Omit the `scanner` field entirely when no status function is configured
    - Register no new route
    - _Requirements: 5.1, 5.2, 5.4_

  - [x] 8.4 Add `apitest` coverage for the health response
    - In `internal/server`, using `handlerTestCase` / `runHandlerTests`: with no
      scanner status configured, `GET /health` returns 200 with `"status":"ok"`
      and no `scanner` field; with a stub status configured, the response
      additionally contains the scanner fields
    - Confirm the existing `handler_webui_test.go` `/health` cases and
      `server_properties_test.go`'s `/health` fallback property still pass
      unmodified
    - _Requirements: 5.1, 5.2, 5.4_

- [x] 9. Configuration and lifecycle wiring in `cmd/server/main.go`
  - [x] 9.1 Add `SCANNER_DEVICE` and `SCAN_INPUT` to `loadScanListenerConfig`
    - Read both via the existing `envOrDefault` helper; `SCANNER_DEVICE` defaults
      to `/dev/pantry-scanner` and sets `DevicePath`; `SCAN_INPUT` goes through
      `scanlistener.ParseSource` and sets `Source`, logging a configuration error
      when `ParseSource` reports the value was unrecognized
    - Log the resolved device path and active source at startup
    - Keep the identical-control-barcode check returning `ok=false` unchanged
    - _Requirements: 7.1, 7.2, 7.3, 7.4_

  - [x] 9.2 Rewire `main()` for the broadcaster and status wiring
    - Construct `events.NewBroadcaster()` in `main`, pass it via
      `server.WithBroadcaster`, and pass `listener.Status` via
      `server.WithScannerStatus` when a listener was configured
    - Keep assigning `listener.Queue = scanQueue` from `NewHandler`'s return —
      not a freshly constructed Queue — so scan events still broadcast, per the
      note in `server.go`
    - Assign `listener.ModePublisher = broadcaster`
    - _Requirements: 1.1, 5.1, 5.3_

  - [x] 9.3 Write unit tests for the configuration changes in
    `cmd/server/main_test.go`
    - `SCANNER_DEVICE` unset yields the default path; an explicit value
      overrides it; use `t.Setenv`, matching the existing `TestProductCacheTTL`
      pattern
    - `SCAN_INPUT` unset yields `SourceDevice`; `stdin` yields `SourceStdin`; an
      unrecognized value yields `SourceDevice`
    - Equal control barcodes still yield `ok=false`
    - _Requirements: 7.1, 7.3, 7.5, 7.6, 7.7_

- [x] 10. Checkpoint - server builds and starts with no scanner attached
  - Confirm the server starts, serves `GET /health` with `connected: false`, and
    logs the resolved device path when `/dev/pantry-scanner` does not exist
  - Ensure all tests pass, ask the user if questions arise.

- [x] 11. Property tests for the capture path
  - [x] 11.1 Write a property test for event filtering
    - New file `internal/scanlistener/assembler_properties_test.go`
    - **Property 1: Only key-press events of the key type affect capture**
    - **Validates: Requirements 1.2, 1.3**
    - Generate a sequence of key-press events, then interleave generated
      releases, auto-repeats, and non-`EV_KEY` records at random positions;
      assert the emitted lines and final mode are identical with and without the
      interleaved noise
    - Minimum 100 iterations, tagged `Feature: headless-scanner-input, Property
      1: Only key-press events of the key type affect capture`
    - _Requirements: 1.2, 1.3_

  - [x] 11.2 Write a property test for Enter-delimited line assembly
    - **Property 2: Line assembly is exactly delimited by Enter**
    - **Validates: Requirements 1.4, 1.6, 1.7**
    - Generate a list of printable-keycode segments, feed them separated by
      Enter, and assert one emitted line per Enter in order, each equal to its
      segment decoded; an empty segment emits `""`; trailing characters after
      the last Enter are retained unemitted
    - Minimum 100 iterations, tagged `Feature: headless-scanner-input, Property
      2: Line assembly is exactly delimited by Enter`
    - _Requirements: 1.4, 1.6, 1.7_

  - [x] 11.3 Write a property test for unmapped-keycode skipping
    - **Property 3: Unmapped keycodes are skipped without corrupting the line**
    - **Validates: Requirements 1.8, 1.9**
    - Minimum 100 iterations, tagged `Feature: headless-scanner-input, Property
      3: Unmapped keycodes are skipped without corrupting the line`
    - _Requirements: 1.8, 1.9_

  - [x] 11.4 Write a property test for shift scoping
    - **Property 4: Shift state applies to exactly the keys pressed while held**
    - **Validates: Requirement 1.5**
    - Generate letter keycodes interleaved with shift press/release events;
      assert each letter is uppercase exactly when a shift was pressed and not
      yet released at that point
    - Minimum 100 iterations, tagged `Feature: headless-scanner-input, Property
      4: Shift state applies to exactly the keys pressed while held`
    - _Requirements: 1.5_

  - [x] 11.5 Write a property test for mode persistence and buffer discard across
    reconnects
    - In `internal/scanlistener/listener_properties_test.go`
    - **Property 5: Mode transitions survive reconnects, partial lines do not**
    - **Validates: Requirements 2.3, 2.6**
    - Generate a sequence of device connections, each delivering generated
      Key_Events and then failing with a device-removal error; assert the final
      `Current_Mode` equals the mode of the last completed Control_Barcode
      across all connections (or `stock_in` if none), and that no created entry's
      barcode is a concatenation spanning two connections
    - Minimum 100 iterations, tagged `Feature: headless-scanner-input, Property
      5: Mode transitions survive reconnects, partial lines do not`
    - _Requirements: 2.3, 2.6_

  - [x] 11.6 Write a property test for backoff bounds
    - **Property 6: Reconnect backoff is bounded and reset by success**
    - **Validates: Requirements 2.4, 2.5**
    - Generate a sequence of open outcomes against an injected clock that records
      every wait; assert each wait is within `[initial, max]`, waits are
      non-decreasing across consecutive failures, and the first wait after any
      success equals the initial backoff
    - Minimum 100 iterations, tagged `Feature: headless-scanner-input, Property
      6: Reconnect backoff is bounded and reset by success`
    - _Requirements: 2.4, 2.5_

  - [x] 11.7 Write a property test for HTTP availability under device failure
    - In `internal/server`, alongside the existing route-set properties
    - **Property 7: Device state never affects HTTP availability**
    - **Validates: Requirements 2.7, 5.2, 5.4**
    - Generate scanner status values, including disconnected-with-error states,
      and assert `GET /health` returns 200 with `"status":"ok"` and the route set
      is unchanged for every one
    - Minimum 100 iterations, tagged `Feature: headless-scanner-input, Property
      7: Device state never affects HTTP availability`
    - _Requirements: 2.7, 5.2, 5.4_

- [x] 12. Checkpoint - full suite and race detector pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 13. Deployment changes
  - [x] 13.1 Add `deploy/udev/99-pantry-scanner.rules`
    - Match `SUBSYSTEM=="input"` with `ATTRS{idVendor}`/`ATTRS{idProduct}`
      placeholders and `KERNEL=="event*"`; create `SYMLINK+="pantry-scanner"`
      with `GROUP="pantry"` and `MODE="0640"`
    - Comment in the file explaining that scoping to one device is what keeps
      Pantry from being able to read every other input device, and that adding
      the service user to the `input` group instead would grant exactly that
    - _Requirements: 4.2, 8.1_

  - [x] 13.2 Update `deploy/docker-compose.yml`
    - Add the `devices:` mapping for `/dev/pantry-scanner`, a `group_add:` entry
      taking a numeric `SCANNER_GID`, and both `SCANNER_DEVICE` and
      `SCAN_INPUT: device` in `environment:`
    - Pin `SCAN_INPUT` explicitly rather than relying on the default, so the
      appliance's intent is stated on the face of the file
    - Remove `stdin_open: true` and `tty: true`
    - Keep the container running as `nonroot` with no added capabilities
    - _Requirements: 4.1, 4.2, 8.2, 8.3_

  - [x] 13.3 Update `deploy/.env.example`
    - Add `SCANNER_GID` and `SCANNER_DEVICE` with comments explaining how to
      obtain the gid and why the path is a udev symlink rather than
      `/dev/input/eventN`
    - _Requirements: 7.1, 8.2_

  - [x] 13.4 Rewrite the scanner sections of `deploy/README.md`
    - Delete the "Barcode Scanner Setup" section's `docker attach` instructions,
      its stdin limitations, and the "Scanner Not Working" stdin troubleshooting
      steps; replace with the udev rule setup, finding the VID/PID via `lsusb`,
      finding the group's numeric gid, and verifying via `GET /health`
    - Delete the "Barcode Scanner Impact" subsection under automatic updates and
      the container-recreation warning in "⚠️ Important Considerations", both of
      which stop being true
    - State that the udev rule must be installed and the device path must exist
      before the new Compose file is applied, because Docker refuses to start a
      container whose declared device is absent
    - State that `pantry-update.service` updates only the container image and
      never the deployment files, so this change needs a one-time manual
      installation on the Pi
    - Add a troubleshooting entry mapping the `scanner` fields in `GET /health`
      to their causes: `connected: false` with a permission error means the udev
      rule's group does not match `SCANNER_GID`; a rising `unmappedKeys` means
      the scanner is emitting keycodes outside the US-layout map
    - _Requirements: 8.3, 8.4, 8.5, 8.6_

  - [x] 13.5 Document the local-development source in `cmd/server/README.md`
    - State that `SCAN_INPUT=stdin go run ./cmd/server` reproduces today's
      behavior — type or scan a barcode into the terminal and press Enter — and
      that this is the only way to exercise control barcodes and mode switching
      off the Pi, since the browser's `POST /api/scans` path bypasses both
    - State that the default is `device`, so omitting `SCAN_INPUT` locally
      produces a listener retrying a device path that does not exist, visible as
      `connected: false` in `GET /health`
    - _Requirements: 10.1, 10.2, 10.3_

- [x] 14. Display the current mode in the web UI
  - [x] 14.1 Consume the scanner-mode SSE event in the frontend
    - Handle the new event type in the existing SSE subscription and show the
      current mode persistently on the scan queue page, so an operator can tell
      stock-in from stock-out without scanning a test item
    - Add a component test alongside the existing `ScanQueuePage` tests
    - _Requirements: 5.3_

- [x] 15. Record the supersession in the `background-scan-listener` spec
  - [x] 15.1 Add a note to
    `.kiro/specs/background-scan-listener/requirements.md` recording that
    Requirements 1.1 through 1.3 are narrowed by `headless-scanner-input` to
    apply only when `SCAN_INPUT=stdin`, that they remain in force under that
    configuration, and that Requirements 2 through 6 are unaffected
    - Prevents a future reader assuming stdin is the default, and prevents the
      never-resume rule in 1.3 being applied to the hot-pluggable device path
    - _Requirements: none (documentation hygiene)_

- [x] 16. Final checkpoint - full suite, race detector, and coverage
  - Run the full suite and confirm every test from tasks 1-14 passes
  - Run `go test -race ./internal/scanlistener/... ./internal/server/...`
  - Run `./scripts/test-coverage.sh` to enforce coverage thresholds and commit the
    updated script if the threshold increased, per AGENTS.md
  - Ensure all tests pass, ask the user if questions arise.

## Manual verification on the Pi

`openEvdev`'s two system calls are the only code this plan leaves untested (Requirement
9.3), so the acceptance check is physical and cannot be asserted by the suite:

0. On the development machine, before touching the Pi: run `SCAN_INPUT=stdin go run
   ./cmd/server`, type a barcode followed by Enter, and confirm the entry appears in the
   queue — then scan the STOCK OUT control barcode and confirm the mode changes. This is
   the regression check that local development still works, and it needs no hardware.
1. Install the udev rule with the real VID/PID, run `sudo udevadm control --reload &&
   sudo udevadm trigger`, and confirm `/dev/pantry-scanner` exists with the expected group.
2. Apply the updated Compose file, then confirm `curl http://localhost:8080/health`
   reports `connected: true` and `grabbed: true`.
3. Scan a product barcode and confirm the entry appears in the scan queue.
4. Scan the STOCK OUT control barcode and confirm the mode changes in `GET /health` and
   in the browser.
5. Unplug the scanner, confirm `connected: false`, replug it, and confirm it returns to
   `connected: true` and still scans — without restarting anything.
6. Reboot the Pi with only power and the scanner attached, wait for boot, and scan. This
   is the requirement in one step: no keyboard, no monitor, no terminal.

## Notes

- Requirement 6 (no new network surface) has no dedicated test task: it is structural,
  enforced by `scanlistener` never importing `net` and by no task here adding a route.
- `background-scan-listener`'s Properties 3, 4, and 5 cover `scan.NewEntryFromLookup`
  and headless attribution, are untouched by this feature, and must keep passing
  unmodified — they are the guard that this change is confined to input capture.
- Tasks 1, 2, and 3 are independent and parallelizable; each is a separate new file with
  no shared dependency.
- Tasks 6.2, 6.3, and 6.6 all edit `listener.go` and are sequenced, not parallelized.
- The stdin path is retained, so any task that would change an existing
  `listener_test.go` assertion (rather than its fixture) indicates a mistake.
- Tasks 8.2 and 8.3 both edit `server.go` and are sequenced.
- Tasks 9.1 and 9.2 both edit `main.go` and are sequenced.
- Task 13 (deployment) has no automated verification and is validated by the manual
  checklist above.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "2.1", "3.1"] },
    { "id": 1, "tasks": ["1.2", "2.2", "3.2", "5.1"] },
    { "id": 2, "tasks": ["5.2", "5.3", "6.1", "6.5", "8.1"] },
    { "id": 3, "tasks": ["6.2"] },
    { "id": 4, "tasks": ["6.3", "8.2"] },
    { "id": 5, "tasks": ["6.6", "8.3"] },
    { "id": 6, "tasks": ["6.4", "6.7", "8.4", "11.1", "11.2", "11.3", "11.4"] },
    { "id": 7, "tasks": ["9.1"] },
    { "id": 8, "tasks": ["9.2"] },
    { "id": 9, "tasks": ["9.3", "11.5", "11.6", "11.7"] },
    { "id": 10, "tasks": ["13.1", "13.2", "13.3", "13.5", "14.1", "15.1"] },
    { "id": 11, "tasks": ["13.4"] }
  ]
}
```
