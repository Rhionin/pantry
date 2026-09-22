# Requirements Document

## Introduction

`ScanListener` currently reads barcodes as lines from the Server_Process's own standard input (`internal/scanlistener/listener.go`). That works when a human runs the server in a focused terminal, because a USB HID scanner types into whatever holds keyboard focus. It does not work on a Raspberry Pi appliance: the deployment in `deploy/` runs the server as a Docker Compose service started by the Docker daemon at boot, where stdin is `/dev/null`. `bufio.Scanner` returns EOF on its first call, `Run` logs `standard input closed, stopping`, and the scan listener exits within milliseconds of every boot. The HTTP server is unaffected, so the failure is silent — the appliance looks healthy and simply never records a scan.

The documented workaround, `sudo docker attach pantry` (see `deploy/README.md`), requires a human with a terminal and an interactive root session held open indefinitely, and it breaks on every container recreation — which `pantry-update.timer` triggers as often as every 5 minutes.

This feature replaces the standard-input capture path with a read of the scanner's own kernel input device (evdev), so that scans are captured with no terminal, no attached session, and no keyboard or monitor ever connected to the Pi. Everything downstream of a completed barcode line — Control_Barcode classification, `Current_Mode`, `scan.NewEntryFromLookup`, `Queue.CreateScanEntry` — is unchanged. This feature also adds the status visibility that a terminal used to provide implicitly, through the existing `GET /health` route and the existing SSE event stream, without adding any HTTP route or network listener.

Standard-input capture is **retained**, not deleted, as an explicitly selected source for local development. A developer running `go run ./cmd/server` in a terminal has no Scanner_Device, no udev rule, and on macOS no evdev subsystem at all, so typing a barcode into the terminal remains the only way to exercise the Control_Barcode and Current_Mode logic outside the Pi. Which source is active is decided by explicit configuration rather than by autodetection, so that a misconfigured appliance fails loudly instead of silently selecting the source that does not work in production.

## Relationship to the `background-scan-listener` spec

This spec supersedes part of `background-scan-listener`:

- **Narrowed, not removed:** Requirements 1.1 and 1.2 (read lines from standard input) remain in force *when the configured Scan_Input_Source is standard input*, which is the local-development configuration (Requirement 10 below). They no longer describe the default or the appliance deployment, which read Key_Events from the Scanner_Device (Requirement 1 below).
- **Narrowed, not removed:** Requirement 1.3 ("SHALL NOT attempt to resume reading" after EOF or a read error) remains in force for the standard-input source and is replaced by Requirement 2 below *for the Scanner_Device source only*. Never resuming is correct for a pipe, which cannot reopen; it is wrong for a hot-pluggable USB device, which routinely disappears and returns. The original rule was not wrong — it was being applied to a source it does not fit.
- **Preserved unchanged:** Requirements 2 (mode switching via control barcodes), 3 (shared queue integration), 4 (attribution), 5 (no new network surface), and 6 (coexistence with browser-based capture). Requirement 5 in particular remains satisfied: this feature exchanges one local file descriptor for another and still binds no port or socket.

Note that an earlier iteration of `background-scan-listener` planned an `internal/scanlistener/device.go` with a `KeyEvent` type (visible in that spec's `tasks.meta.json` execution history) and was revised to the standard-input design, on the stated grounds that stdin left "no hardware-only or platform-only file" unfaked. That tradeoff was sound when a focused terminal was available. It is not available on this appliance, so Requirement 9 below carries the testability obligation forward explicitly: the decoding and line-assembly logic must remain fully testable without hardware, and only a thin device-opening shell may be untestable in a normal `go test` run.

## Glossary

- **Scan_Input_Source**: Which of two origins the ScanListener reads barcodes from — the Scanner_Device (the appliance default) or the Server_Process's own standard input (local development). Selected by explicit configuration, never autodetected.
- **Scanner_Device**: The kernel input (evdev) character device that a USB HID barcode scanner presents, from which Key_Events are read.
- **Device_Path**: The filesystem path at which the Server_Process opens the Scanner_Device. A udev-managed stable symlink, not a bare `/dev/input/eventN` path, which is not stable across reboots or replugs.
- **Key_Event**: One `input_event` record read from the Scanner_Device, carrying an event type, a keycode, and a value distinguishing press, release, and auto-repeat.
- **Scan_Line**: The in-progress buffer of decoded characters that the ScanListener accumulates between Enter keys; a completed Scan_Line is one barcode value.
- **Exclusive_Grab**: The `EVIOCGRAB` ioctl, which makes the Server_Process the sole consumer of the Scanner_Device's events for as long as it holds the device open.
- **Reconnect_Backoff**: The growing, capped delay the ScanListener waits between attempts to open a Device_Path that is absent or failing.
- **Scanner_Status**: The externally observable state of the capture path — whether the Scanner_Device is currently open, whether Exclusive_Grab is held, the Current_Mode, and when a scan was last accepted.
- **ScanListener**, **Control_Barcode**, **Current_Mode**, **Product_Barcode**, **Queue**, **Headless_User_ID**, **Server_Process**: as defined in the `background-scan-listener` spec.

## Requirements

### Requirement 1: Scan capture from the scanner's input device

**User Story:** As a pantry owner, I want the Pi to capture scans with nothing attached but power and the scanner, so that I never need a keyboard, a monitor, or a terminal session to stock items in or out.

Every criterion in this requirement applies WHERE the configured Scan_Input_Source is the Scanner_Device.

#### Acceptance Criteria

1. THE Server_Process SHALL start, at Server_Process startup, a ScanListener that reads Key_Events from the Scanner_Device at the configured Device_Path, and SHALL NOT read scan input from the Server_Process's standard input.
2. THE ScanListener SHALL process only Key_Events of the key-event type, and SHALL discard every Key_Event of any other type without altering the Scan_Line buffer or the Current_Mode.
3. THE ScanListener SHALL process only key-press Key_Events, and SHALL discard key-release and auto-repeat Key_Events without altering the Scan_Line buffer or the Current_Mode.
4. WHEN the ScanListener processes a key-press Key_Event whose keycode maps to a printable character, THE ScanListener SHALL append that character to the Scan_Line buffer.
5. WHILE a modifier keycode designating shift is held down, THE ScanListener SHALL append the shifted form of a subsequent printable keycode, and WHEN that modifier is released THE ScanListener SHALL resume appending unshifted forms.
6. WHEN the ScanListener processes a key-press Key_Event whose keycode designates Enter, THE ScanListener SHALL treat the accumulated Scan_Line buffer as one complete barcode value, SHALL clear the buffer, and SHALL pass that value through the same Control_Barcode classification and entry-creation path the standard-input implementation used.
7. IF a completed Scan_Line contains zero characters, THEN THE ScanListener SHALL discard it and SHALL NOT create a scan entry from it.
8. IF a key-press Key_Event's keycode has no mapping to a printable character and does not designate Enter or a shift modifier, THEN THE ScanListener SHALL discard that Key_Event, SHALL log the unmapped keycode, and SHALL leave the Scan_Line buffer intact.
9. THE ScanListener SHALL count discarded unmapped keycodes and SHALL expose that count through Scanner_Status, so that a scanner configured for an unexpected keyboard layout is diagnosable without a monitor attached.

### Requirement 2: Device availability and reconnection

**User Story:** As a pantry owner, I want the Pi to work when I plug it in and to recover when I unplug and replug the scanner, so that it keeps working without me logging in to restart anything.

Every criterion in this requirement applies WHERE the configured Scan_Input_Source is the Scanner_Device. The standard-input source keeps `background-scan-listener` Requirement 1.3's behavior instead: on end-of-file or a read error it logs, stops, and does not resume, because a closed pipe cannot reopen.

#### Acceptance Criteria

1. IF the Device_Path does not exist or cannot be opened when the ScanListener starts, THEN THE Server_Process SHALL log that condition, SHALL NOT fail startup, SHALL continue serving all existing HTTP routes, and THE ScanListener SHALL retry opening the Device_Path.
2. WHEN a read from the Scanner_Device fails because the device was removed, THE ScanListener SHALL log that condition and SHALL return to retrying the Device_Path rather than terminating.
3. WHEN the ScanListener returns to retrying the Device_Path, THE ScanListener SHALL discard any partially accumulated Scan_Line buffer, so that no barcode value is ever assembled from characters captured across two separate device connections.
4. THE ScanListener SHALL wait a Reconnect_Backoff between successive open attempts, SHALL increase that delay on repeated consecutive failures, and SHALL cap it at a configured maximum so that an absent scanner does not fill the journal.
5. WHEN an open attempt succeeds after one or more failures, THE ScanListener SHALL log that it reconnected and SHALL reset the Reconnect_Backoff to its initial value.
6. WHEN the ScanListener reconnects to the Scanner_Device, THE ScanListener SHALL preserve the Current_Mode that was in effect before the disconnection, since a cable interruption is not an operator instruction to change direction.
7. THE Server_Process SHALL continue to serve every existing HTTP route unaffected, whether the Scanner_Device is connected, absent, or failing.
8. THE ScanListener SHALL stop its retry loop when its context is cancelled.

### Requirement 3: Exclusive access to the scanner

**User Story:** As a pantry owner, I want scanned barcodes to reach only Pantry, so that they are not also typed into the Pi's console as login attempts.

#### Acceptance Criteria

1. WHEN the ScanListener opens the Scanner_Device, THE ScanListener SHALL request Exclusive_Grab on that device.
2. IF the Exclusive_Grab request fails, THEN THE ScanListener SHALL log that failure, SHALL continue reading Key_Events from the device without the grab, and SHALL report through Scanner_Status that the grab is not held, since capture still functions in that state.
3. WHEN the ScanListener closes the Scanner_Device, for either a read failure or a cancelled context, THE ScanListener SHALL release the Scanner_Device.

### Requirement 4: Least privilege for device access

**User Story:** As a developer, I want the scan capture path to read exactly one device and nothing more, so that adding it does not let Pantry observe keystrokes from any other input device.

#### Acceptance Criteria

1. THE Server_Process SHALL NOT require running as the root user in order to read the Scanner_Device.
2. THE Server_Process SHALL require read access to only the single configured Scanner_Device, and SHALL NOT require membership in any group that grants read access to input devices other than the Scanner_Device.
3. THE ScanListener SHALL open the Scanner_Device read-only.

### Requirement 5: Status visibility without a terminal

**User Story:** As a pantry owner, I want to see from my phone whether the scanner is working and which mode it is in, so that I can tell a dead scanner from an idle one without connecting a monitor.

#### Acceptance Criteria

1. WHEN a client issues GET /health, THE Server_Process SHALL include in the response the Scanner_Status: the active Scan_Input_Source, whether the Scanner_Device is currently open, the configured Device_Path, whether Exclusive_Grab is held, the Current_Mode, the count of discarded unmapped keycodes, and the time at which a barcode was last accepted.
2. THE Server_Process SHALL continue to include in the GET /health response the existing `"status":"ok"` field, with its existing value, so that existing health checks and the documented `curl http://localhost:8080/health` verification step continue to pass unchanged.
3. WHEN the ScanListener changes the Current_Mode in response to a Control_Barcode, THE Server_Process SHALL publish that change on the existing server-sent-events stream, so that a browser already subscribed observes the new mode without polling.
4. THE Server_Process SHALL NOT register any HTTP route beyond the set of HTTP routes registered prior to this feature.

### Requirement 6: No new network surface

**User Story:** As a developer, I want the headless capture path to remain free of network exposure, so that it does not create an unauthenticated remote entry point.

#### Acceptance Criteria

1. THE ScanListener SHALL read scan input exclusively from the configured Scan_Input_Source — either the Scanner_Device or the Server_Process's own standard input, both of which are local file descriptors — and SHALL NOT accept scan input from any network connection under either configuration.
2. THE ScanListener SHALL NOT open, bind, or listen on any TCP port, UDP port, or Unix domain socket to accept incoming connections.

### Requirement 7: Configuration

**User Story:** As a pantry owner, I want the device path and control barcodes to be settable in the same `.env` file as everything else, so that configuration stays in one place.

#### Acceptance Criteria

1. THE Server_Process SHALL read the Device_Path from an environment variable, defaulting to a udev-managed stable symlink path when that variable is unset or empty.
2. THE Server_Process SHALL continue to read the stock-in Control_Barcode, stock-out Control_Barcode, and Headless_User_ID from their existing environment variables with their existing defaults.
3. IF the configured stock-in Control_Barcode and stock-out Control_Barcode are identical, THEN THE Server_Process SHALL log a configuration error and continue serving existing HTTP routes without a ScanListener.
4. THE Server_Process SHALL log the resolved Device_Path and the active Scan_Input_Source at startup, so that a misconfigured source or path is diagnosable from the journal alone.
5. THE Server_Process SHALL read the Scan_Input_Source from an environment variable accepting exactly two values, one selecting the Scanner_Device and one selecting standard input.
6. WHERE the Scan_Input_Source variable is unset or empty, THE Server_Process SHALL select the Scanner_Device, so that the appliance deployment requires no configuration to work and no typo can silently select the source that does not function under systemd.
7. IF the Scan_Input_Source variable holds a value that is neither of the two accepted values, THEN THE Server_Process SHALL log a configuration error naming the offending value and SHALL select the Scanner_Device, rather than guessing or failing startup.

### Requirement 8: Deployment on the Raspberry Pi appliance

**User Story:** As a pantry owner, I want a documented one-time setup that leaves the Pi working across reboots and automatic updates, so that I plug it in and it Just Works from then on.

#### Acceptance Criteria

1. THE deployment SHALL provide a udev rule that matches the scanner by vendor and product identifier, creates the stable Device_Path symlink, and assigns group ownership granting read access to the group the container process runs under.
2. THE deployment SHALL pass the Device_Path into the container and SHALL grant the container process the group needed to read it, without granting the container additional device access.
3. THE deployment SHALL NOT retain the `stdin_open` or `tty` settings, and the deployment documentation SHALL NOT instruct the operator to attach to the container's standard input.
4. WHEN `pantry-update.service` recreates the container, THE recreated container SHALL retain scanner capture with no operator action, since capture no longer depends on an attached session.
5. THE deployment documentation SHALL state that the udev rule must be installed and the Device_Path must exist before the Compose file declaring that device is applied, because the container will fail to start if the declared device path is absent.
6. THE deployment documentation SHALL state that `pantry-update.service` updates only the container image and never the deployment files, so the Compose and udev changes for this feature require a one-time manual installation on the Pi.

### Requirement 9: Testability of the capture path

**User Story:** As a developer, I want the decoding and reconnection logic covered by automated tests, so that reintroducing device access does not reintroduce untested code.

#### Acceptance Criteria

1. THE keycode decoding and Scan_Line assembly logic SHALL be exercisable in a normal `go test` run against synthetic Key_Event bytes, with no Scanner_Device present.
2. THE reconnection and backoff logic SHALL be exercisable in a normal `go test` run against an injected device opener that fails, succeeds, and fails again on demand, with no Scanner_Device present.
3. THE only logic permitted to be unexercisable without hardware SHALL be the concrete system calls that open a device path and request Exclusive_Grab, isolated behind an injectable seam.


### Requirement 10: Local development via standard input

**User Story:** As a developer, I want to keep typing barcodes into my terminal when I run the server locally, so that I can exercise control barcodes, mode switching, and the scan queue without a Raspberry Pi, a udev rule, or a scanner plugged into my laptop.

#### Acceptance Criteria

1. WHERE the configured Scan_Input_Source is standard input, THE ScanListener SHALL read complete lines from the Server_Process's own standard input and SHALL treat the full content of each line as a single barcode value, with the behavior `background-scan-listener` Requirements 1.1 through 1.4 specify.
2. WHERE the configured Scan_Input_Source is standard input, THE ScanListener SHALL NOT open, read, or require the existence of the Scanner_Device, so that a development machine with no evdev subsystem is fully supported.
3. WHERE the configured Scan_Input_Source is standard input, THE ScanListener SHALL apply the same Control_Barcode classification, Current_Mode handling, product lookup, and Queue.CreateScanEntry path it applies to barcodes read from the Scanner_Device, so that a barcode typed in a terminal and a barcode scanned on the Pi are indistinguishable downstream.
4. WHERE the configured Scan_Input_Source is standard input AND standard input is not a terminal, THE Server_Process SHALL log a warning identifying that combination, SHALL report it through Scanner_Status, and SHALL continue running, because that combination is the silent-failure mode this feature exists to eliminate: it is what a systemd or container deployment produces when misconfigured.
5. THE Server_Process SHALL determine whether standard input is a terminal by a check that distinguishes a terminal from a character device that is not a terminal, since the standard input a service manager supplies is `/dev/null`, which is itself a character device.
6. THE ScanListener SHALL select the Scan_Input_Source once at startup and SHALL NOT switch sources while running, so that the active source is a fixed, reportable fact rather than a race against device availability.
7. THE existing automated tests covering the standard-input reading path SHALL continue to pass unmodified, since that path's behavior is unchanged by this feature.
