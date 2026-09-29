# Scanner Hot-Plug Resilience Bugfix Design

## Overview

`deploy/docker-compose.yml` declares the scanner as a container device:

```yaml
devices:
  - /dev/pantry-scanner:/dev/pantry-scanner
```

Docker resolves that path at container **create** time and refuses to create the container if it is absent. `/dev/pantry-scanner` is a udev symlink that exists only while the scanner is enumerated, so any create attempt with the scanner unplugged, asleep, or not yet enumerated fails. That single declaration is the entire boot failure.

The fix removes the create-time requirement rather than trying to satisfy it. The container bind-mounts the `/dev/input` **directory** instead of one node inside it. A directory bind mount shares the underlying devtmpfs directory, so nodes udev creates later appear inside the container without the container being recreated. A `device_cgroup_rules` entry grants the cgroup permission to read char-major-13 (the Linux input class), and the udev `SYMLINK` moves to `input/pantry-scanner` so the stable name lives inside the mounted directory.

No application change is required. `ScanListener.runDevice` (`internal/scanlistener/listener.go`) already loops on `Open` with backoff, and `cmd/server/main.go` starts the listener in a goroutine before `http.ListenAndServe`, so a missing device already degrades gracefully and a device appearing later is already picked up. This spec's Go changes are limited to aligning the compiled-in default device path and pinning the tolerance behavior with tests.

Per-device scoping is unchanged in effect but changes in mechanism, and the docs must say so: previously the container could see exactly one node; now it can see the whole `/dev/input` directory, and what prevents it reading other devices is that those nodes are `root:input` mode `0600` while the udev rule grants the scanner `GROUP="65532"` mode `0640`. The container process runs as uid 65532 with supplementary group 65532, so it can open the scanner and nothing else.

## Glossary

- **Bug_Condition (C)**: A container create is attempted while the scanner's udev symlink does not exist on the host.
- **Property (P)**: The container is created and starts successfully, serves HTTP, and reports the scanner as disconnected, regardless of whether the scanner is present.
- **Preservation**: All behavior observed when the scanner *is* present and readable — scan ingestion, exclusive grab, control-barcode mode switching, stdin development mode, database persistence — is unchanged.
- **Hot-plug**: Connecting or disconnecting the scanner while the container is running, in either direction, any number of times.
- **Device node**: The kernel-created character device, `/dev/input/eventN`, char major 13.
- **Stable symlink**: The udev-created alias for the scanner's node, currently `/dev/pantry-scanner`, becoming `/dev/input/pantry-scanner`.
- **`device_cgroup_rules`**: A Compose key that adds device cgroup allowances to the container. An allowance, not a grant — file ownership and mode still decide what is openable.
- **Reconnect loop**: The `for ctx.Err() == nil` loop in `ScanListener.runDevice` that retries `Open` with backoff and resets the backoff on success.

## Bug Details

### Bug Condition

The bug manifests on every container create where the scanner's symlink is absent. It is a property of the host at create time, not of any input the application receives, which is why it is invisible to the Go test suite.

**Formal Specification:**
```
FUNCTION isBugCondition(host, compose)
  INPUT: host  - the host filesystem state at container-create time
         compose - the compose service definition
  OUTPUT: boolean

  // Docker resolves every entry in `devices:` when creating the container.
  FOR EACH declared IN compose.devices DO
    IF NOT exists(host, declared.hostPath) THEN
      RETURN TRUE   // `docker compose up` fails; no container exists
    END IF
  END FOR

  RETURN FALSE
END FUNCTION
```

With `compose.devices` empty — the state after this fix — `isBugCondition` is unsatisfiable by construction. That is the point: the fix eliminates the condition rather than handling it.

### Examples

- The Pi reboots overnight with the scanner unplugged for charging. Current behavior: `docker compose up -d` fails with a device-not-found error, no container exists, and the web UI is unreachable from any device on the network. Expected: the container starts, the UI serves, `/health` reports `scanner.connected: false`.
- `pantry-update.timer` fires while the scanner is disconnected and recreates the container for a new image. Current behavior: the recreate fails and takes a previously working deployment down. Expected: the recreate succeeds and the scanner is picked up whenever it returns.
- The operator plugs the scanner into a running Pi. Current behavior: irrelevant, because the container could not have started without it. Expected: the reconnect loop's next `Open` succeeds and scanning works within 30 seconds, with no container action.
- A Bluetooth scanner sleeps to save power and drops its connection. Current behavior: the node disappears; the container keeps running but a later recreate would fail. Expected: the node disappears, `scanner.connected` goes false, the UI stays up, the container is untouched, and scanning resumes on wake.
- Edge case: at boot the container may start before udev has created the symlink. This must NOT be treated as an error — the reconnect loop absorbs it, resolving on its own within 30 seconds rather than needing a restart.
- Edge case: `/dev/input` does not exist on a host that has never had an input device attached. A short-syntax bind mount would cause Docker to create the source as an ordinary directory on the root filesystem, which will never reflect devtmpfs nodes, so the scanner would silently never be found. The setup script must ensure the directory exists first.

## Expected Behavior

### Preservation Requirements

**Unchanged Behaviors:**
- With the scanner connected and readable, barcodes are read from its evdev node and enqueued exactly as today.
- The exclusive grab (`EVIOCGRAB`) is still requested on open, and a failed grab is still non-fatal — capture continues without it.
- `STOCK_IN` / `STOCK_OUT` control barcodes still switch scanner mode instead of enqueuing a product scan.
- Input devices other than the designated scanner remain unreadable by the container.
- The container still runs as uid 65532 and still runs unprivileged.
- `SCAN_INPUT=stdin` still reads scans from standard input and still needs no device.
- The `pantry-data` named volume still preserves the database across container recreation.
- A pinned `PANTRY_IMAGE_TAG` still deploys that image and is never silently reset.
- `ADDR` and `DB_PATH` remain pinned by the compose file and are not made overridable.

**Scope:**
Every input where the scanner is present and readable should be completely unaffected by this fix. The change is confined to how the container is granted access to the device, not to how scans are read, decoded, classified, or stored. No handler, no queue method, and no decoding logic is touched.

## Hypothesized Root Cause

1. **A hot-pluggable resource was declared as a static one.** `devices:` is the correct mechanism for a device that is always present at create time. The scanner is not: it is a USB or Bluetooth peripheral that appears and disappears. The declaration encodes an assumption the hardware does not honor.
2. **The declaration duplicates a guarantee the application already provides.** The reconnect loop was built precisely to handle an absent device, so nothing above the Docker layer needs the path to exist. The `devices:` entry adds a hard precondition on top of a component designed to tolerate its absence, and the hard precondition is what fails.
3. **A single node was mounted where a directory was needed.** Bind-mounting one node freezes the container's view at create time. Bind-mounting the containing directory keeps the view live, which is what a hot-pluggable device requires.

## Rejected Alternative

The `feature/udev-driven-container-lifecycle` branch (PR #10, now closed) took the opposite approach: keep the `devices:` declaration and instead gate container start on a udev-driven `pantry.service` systemd unit, with `restart: on-failure` and `BindsTo=dev-pantry\x2dscanner.device`. It is rejected for two independent reasons.

**It does not work as written.** The unit binds to `dev-pantry\x2dscanner.device`, a device unit that is never created. Per [systemd.device(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.device.html), device units are named after the device node the kernel exposes (the scanner's unit is `dev-input-eventN.device`), and an additional alias requires the `SYSTEMD_ALIAS` property: "Adds an additional alias name to the device unit. This must be an absolute path that is automatically transformed into a unit name." The branch's `SYMLINK+="pantry-scanner"` does not create an alias, and the branch never sets `SYSTEMD_ALIAS`, so `BindsTo` references a unit that is never active and the service is stopped immediately after udev starts it. *(Documentation content rephrased for licensing compliance.)*

**Even fixed, it inverts the goal.** `BindsTo` stops the container when the device goes away, so the web UI would go dark whenever the scanner sleeps or drops off Bluetooth — a new failure mode indistinguishable, to a user, from the one being fixed. The same man page also notes that `Wants=` dependencies are only acted on when a device first becomes active, not when added to an already-active device, which makes re-triggering after a rule edit unreliable and iteration confusing.

Design B needs no systemd unit, no device-unit alias, and no `SYSTEMD_WANTS` semantics. Implementations must not reintroduce them.

## Correctness Properties

Property 1: Bug Condition - Container Starts Without The Scanner

_For any_ container create attempted while the scanner's symlink is absent, the fixed deployment SHALL create and start the container, serve the web UI, and report `status: ok` with `scanner.connected: false` from `GET /health`.

**Validates: Requirements 2.1, 2.2**

Property 2: Hot-Plug Convergence - Scanner State Follows The Hardware

_For any_ sequence of connect and disconnect events on a running container, the system SHALL converge within 30 seconds to `scanner.connected: true` while the device is present and readable and to `scanner.connected: false` while it is absent, SHALL serve the web UI throughout, and SHALL NOT stop, restart, or recreate the container at any point in the sequence.

**Validates: Requirements 2.3, 2.4, 2.5**

Property 3: Preservation - Scan Ingestion With The Scanner Present

_For any_ scan taken while the scanner is connected and readable, the fixed deployment SHALL produce exactly the same result as before the fix: barcodes decoded and enqueued identically, the exclusive grab still requested and still non-fatal on failure, control barcodes still switching mode, and stdin mode still working with no device.

**Validates: Requirements 3.1, 3.2, 3.3, 3.6**

Property 4: Preservation - Access Remains Scoped To One Device

_For any_ set of input devices attached to the host, the container SHALL be able to open only the node the udev rule granted `GROUP="65532"` mode `0640`, SHALL remain unable to read any other input node, and SHALL remain unprivileged and running as uid 65532.

**Validates: Requirements 3.4, 3.5**

Property 5: Preservation - Configuration And Data Survive Convergence

_For any_ number of repeated setup-script runs, every value already present in `/opt/pantry/.env` SHALL be unchanged, a pinned `PANTRY_IMAGE_TAG` SHALL still be the deployed image, and the `pantry-data` volume SHALL still hold the database.

**Validates: Requirements 2.6, 3.7, 3.8**

## Fix Implementation

### Changes Required

**File**: `deploy/docker-compose.yml`

**Specific Changes**:

1. **Remove the `devices:` block.** Delete both lines:
   ```yaml
   devices:
     - /dev/pantry-scanner:/dev/pantry-scanner
   ```
   This is the change that fixes the defect. Do not replace it with another `devices:` entry.

2. **Bind-mount the input directory.** Add a second entry to the existing `volumes:` list:
   ```yaml
   volumes:
     - pantry-data:/data
     - /dev/input:/dev/input:ro
   ```
   A directory bind mount shares the underlying devtmpfs directory, so nodes created later are visible without recreating the container. `:ro` is safe because `openEvdev` (`internal/scanlistener/device.go`) opens with `unix.O_RDONLY`, and a read-only mount does not block the `EVIOCGRAB` ioctl.

3. **Grant the input device class at the cgroup layer.** Add:
   ```yaml
   device_cgroup_rules:
     - 'c 13:* r'
   ```
   Char major 13 is the Linux input class. Docker's default device cgroup denies devices that were not explicitly added, so without this the container cannot open the node even though it can see it. This is an allowance, not a grant — file ownership and mode still decide what is openable.

4. **Make the device path overridable and point it at the new symlink.** Change `SCANNER_DEVICE: /dev/pantry-scanner` to `SCANNER_DEVICE: ${SCANNER_DEVICE:-/dev/input/pantry-scanner}`. Overridability is a deliberate debugging affordance: pointing straight at a concrete `/dev/input/eventN` isolates udev problems from container problems.

5. **Leave `restart: unless-stopped` and `group_add:` unchanged.** The container must still start at boot, and `group_add` is what puts the process in GID 65532 so it can read the `0640` node.

**File**: `deploy/udev/99-pantry-scanner.rules`

**Specific Changes**:

1. **Move the symlink inside the mounted directory.** Change `SYMLINK+="pantry-scanner"` to `SYMLINK+="input/pantry-scanner"` in both examples. This is a requirement, not a preference: udev writes a *relative* symlink, so `/dev/pantry-scanner -> input/eventN` would not exist inside a container that mounts only `/dev/input`, and its target would not resolve there either.

2. **Match on `ENV{ID_VENDOR_ID}` / `ENV{ID_MODEL_ID}`** for USB scanners instead of `ATTRS{idVendor}` / `ATTRS{idProduct}`. udev's `input_id` builtin sets those properties directly on the event device, whereas `ATTRS{}` must walk up the parent chain to the USB node and is order-sensitive. Keep `ATTRS{name}` for Bluetooth, since that attribute is on the input device itself.

3. **Keep `GROUP="65532"` and `MODE="0640"`,** which are now the sole mechanism scoping access to one device, and update the header comment to say so rather than implying the container sees only one node.

4. **Add no systemd integration.** No `TAG+="systemd"`, no `ENV{SYSTEMD_WANTS}`, no `ENV{SYSTEMD_ALIAS}`, and no `pantry.service`.

**Files**: `deploy/.env.example`, `cmd/server/main.go`, `internal/scanlistener/listener.go`

**Specific Changes**:

Align the documented and compiled-in defaults with the new path. `SCANNER_DEVICE` in `.env.example`; the `envOrDefault("SCANNER_DEVICE", ...)` call and its doc comment in `cmd/server/main.go`; and the `DevicePath` struct default in `internal/scanlistener/listener.go`. Two assertions in `internal/server/handler_scan_headless_test.go` reference the old literal and must be updated with it. No logic changes.

**File**: `deploy/setup.sh`

The existing file is an unshipped draft and is rewritten rather than patched. It must not be carried forward as-is: it overwrites `/opt/pantry/.env` on every run (resetting a pinned `PANTRY_IMAGE_TAG` and then redeploying `latest`, defeating the documented rollback procedure and violating requirement 3.8), never copies the `systemd/` and `udev/` subdirectories it later reads from, and writes a literal unexpanded `$(dpkg --print-architecture)` into an apt sources file.

The rewrite is subcommand-driven because the operational need is iteration, not one-shot install: `install`, `rule`, `status`, `logs`, `freeze`, `thaw`, `help`. The highest-value part is `status`, which walks the chain from udev rule to scan ingestion and names the broken link — satisfying requirement 2.7 and turning a multi-minute guess into a single command.

## Testing Strategy

### Validation Approach

This bug does not live in Go code, and that shapes the whole strategy. The bug condition is a Docker create-time check against host filesystem state, so it cannot be reproduced by any Go test, and it cannot be reproduced in the sandbox at all — there is no compose plugin, no `udevadm`, and no scanner. Pretending otherwise would produce tests that assert nothing.

The work therefore splits in two:

- **In the sandbox**, lock in *preservation*: prove the application tolerates an absent device, reports it honestly, and reconnects when it appears, so the deployment change is provably safe rather than hopefully safe. These are the tests that would catch a future regression in the tolerance this fix now depends on.
- **On hardware**, prove the *fix*: only a Pi can demonstrate that the container starts with the scanner unplugged. This is captured as an ordered acceptance runbook, `deploy/ACCEPTANCE.md`, executed by the operator.

No task may report a hardware check as passed from the sandbox.

### Exploratory Bug Condition Checking

**Goal**: Demonstrate the defect before fixing it.

**Status**: NOT REPRODUCIBLE IN THE SANDBOX. The reproduction is `docker compose up -d` on a host where `/dev/pantry-scanner` does not exist, which requires the compose plugin and a real devtmpfs. It is recorded as check T1 in `deploy/ACCEPTANCE.md` and is the headline acceptance test.

**Expected Counterexample**: `docker compose up -d` fails to create the container, citing the missing device path, and no container exists afterward. The web UI is unreachable.

### Preservation Checking

**Goal**: Verify that everything observable with the scanner present is unchanged, and that the missing-device tolerance this fix relies on actually holds.

**Test Plan**: Exercise `ScanListener` through its injectable `Open OpenFunc` field with the unexported `initialBackoff` / `maxBackoff` fields set to microseconds, so retry behavior is testable without touching real devices or real time. Observe on current code first, then assert the observed behavior.

**Test Cases**:
1. **Open always fails**: status reports `connected: false` with a non-nil `lastError`, and the loop keeps retrying rather than returning.
2. **Open fails N times then succeeds**: status transitions to `connected: true`, and the backoff resets to its initial value on success.
3. **Device disappears mid-read**: the read returns, status returns to `connected: false`, and the loop resumes retrying rather than exiting.
4. **HTTP is unaffected**: `GET /health` returns success and reports `scanner.connected: false` when the configured device path does not exist.
5. **Exclusive grab still non-fatal**: an `Open` reporting a failed grab still yields a working reader and `grabbed: false`.
6. **stdin mode still needs no device**: with `SCAN_INPUT=stdin`, scans are read from standard input with no device path consulted.

### Unit Tests

- The reconnect loop retries on `Open` error and resets backoff on success (cases 1-3 above).
- `nextBackoff` doubles and saturates at `maxBackoff`, so the worst-case pickup delay is bounded at the documented 30 seconds.
- The compiled-in default `DevicePath` is `/dev/input/pantry-scanner`.

### Integration Tests

- `GET /health` returns `status: ok` and a scanner object reporting `connected: false` and the configured device path, when that path does not exist. Use the `handlerTestCase` / `runHandlerTests` framework in `internal/server`, following `handler_scan_headless_test.go`; assert against literal expected values.
- Control-barcode mode switching and scan enqueueing continue to pass unchanged, via the existing suites in `internal/scanlistener` and `internal/server`.

### Hardware Acceptance Checking

Captured as `deploy/ACCEPTANCE.md` with one ordered check each for: compose schema validation on the Pi (closing the sandbox gap); boot with the scanner unplugged (the headline test); hot-plug pickup within 30 seconds; hot-unplug leaving the UI up and the container *not* recreated, evidenced by an unchanged `docker inspect -f '{{.State.StartedAt}}'`; replug recovery; boot with the scanner attached; clean install from scratch; a second `install` preserving a pinned image tag; host-side permission evidence that other input nodes stay unreadable; `udevadm` and `shellcheck` runs; and the update-timer freeze/thaw controls.

The runbook must also distinguish the three failure modes that look alike in `/health`'s `lastError` and have different fixes, since getting this wrong wastes the most time:

| Error | Meaning | Fix |
|---|---|---|
| `ENOENT` / no such file | udev rule did not match; no symlink created | fix the rule's match keys |
| `EACCES` / permission denied | node's group is not 65532 | fix `GROUP` in the rule or `SCANNER_GID` in `.env` |
| `EPERM` / operation not permitted | device cgroup denied the open | `device_cgroup_rules` is not taking effect |

`EPERM` is the only observation that would invalidate this design. If it occurs, re-test with the rule widened to `'c 13:* rwm'` to determine whether the mode string is at fault and record the result. Do not fall back to `privileged: true`, and do not reintroduce a `devices:` entry — that restores the original defect. If the cgroup approach genuinely cannot work on the target kernel, stop and escalate with the evidence.

Note that the image is distroless, so `docker exec pantry sh` does not work. All runtime introspection is via `docker compose logs pantry`, `GET /health`, and `sudo ./setup.sh status`.
