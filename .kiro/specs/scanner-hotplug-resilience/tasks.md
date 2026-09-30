# Implementation Plan

## Overview

This plan makes Pantry boot and serve headlessly whether or not the barcode scanner is
attached, by removing the create-time device requirement from `deploy/docker-compose.yml`
rather than by gating container start on the device appearing. It follows the exploratory
bugfix workflow with one deviation forced by the nature of the defect: the bug condition is
a Docker create-time check against host filesystem state, so it is **not reproducible in
Go tests or in this sandbox**. Task 1 therefore locks in *preservation* — the application's
existing tolerance of an absent device, which the fix depends on — and the bug-condition
demonstration is deferred to hardware as check T1 in `deploy/ACCEPTANCE.md` (Task 6).

Per `AGENTS.md`, Go tests live alongside their packages (`internal/scanlistener/listener_test.go`
using the injectable `Open OpenFunc` field and the unexported `initialBackoff`/`maxBackoff`
fields; `internal/server` using the `handlerTestCase` / `runHandlerTests` framework from
`test_runner_test.go` with `exchanges()` for follow-up assertions). Coverage is enforced by
`./scripts/test-coverage.sh`, whose threshold is currently `74.9`.

The **Bug Condition** is `exists(/dev/pantry-scanner) == false` at container-create time
(design "Bug Details"). The fix makes it unsatisfiable by removing the `devices:` declaration
entirely, replacing it with a `/dev/input` directory bind mount plus a `device_cgroup_rules`
allowance for char-major-13, and moving the udev `SYMLINK` inside the mounted directory.

Work on the `feature/scanner-hotplug-resilience` branch, which already exists and already
contains this spec. Do not commit to `master`.

**Do not**, at any point: add a `pantry.service` unit, add `TAG+="systemd"`,
`ENV{SYSTEMD_WANTS}`, or `ENV{SYSTEMD_ALIAS}`; branch from or cherry-pick from
`feature/udev-driven-container-lifecycle`; change `restart: unless-stopped`; use
`privileged: true`; reintroduce a `devices:` entry; delete an existing test or `log.Printf`
statement; or modify anything under `frontend/` (`npm` is unavailable in this sandbox).
See design "Rejected Alternative" for why the systemd approach is excluded.

---

## Tasks

- [ ] 1. Write preservation tests for missing-device tolerance (BEFORE changing the deployment)

  - [ ] 1.1 Record the baseline and the reconnect contract
    - **IMPORTANT**: Follow observation-first methodology — run the current code, record
      what it actually does, then encode those observations as assertions
    - From the repo root run `go build ./...`, `go test ./...`, and `./scripts/test-coverage.sh`;
      record each result and the reported coverage percentage. Do NOT run frontend tests
    - Read `internal/scanlistener/listener.go` and record the reconnect contract with line
      citations: the `runDevice` loop, `defaultInitialBackoff = 250 * time.Millisecond`,
      `defaultMaxBackoff = 30 * time.Second`, the doubling in `nextBackoff`, and the reset to
      initial on a successful open. State the resulting worst-case pickup delay
    - Read `cmd/server/main.go` and record that `go listener.Run(context.Background())`
      (line ~121) precedes `http.ListenAndServe` (line ~125), so an unopenable device cannot
      block or fail HTTP startup
    - Inventory which of the following are ALREADY covered by existing tests, naming each
      test: retry on `Open` error, status reporting `connected: false` with a `lastError`,
      and successful open after prior failures
    - **EXPECTED OUTCOME**: baseline is green; the reconnect contract and existing coverage
      are documented
    - _Requirements: 2.2, 2.3_

  - [ ] 1.2 Add reconnect-loop preservation tests
    - **Property 3: Preservation** - Scan Ingestion And Tolerance With A Missing Device
    - **GOAL**: Lock in the tolerance the fix depends on, so a future change cannot silently
      remove it
    - Add only the cases found uncovered in 1.1 — if a case is already covered, say so and
      add nothing rather than duplicating it
    - In `internal/scanlistener/listener_test.go`, inject an `Open` func via the `Open OpenFunc`
      field and set `initialBackoff`/`maxBackoff` to microseconds so the tests are fast:
      - **Open always fails**: status reports `connected: false` with a non-nil `lastError`,
        and the loop keeps retrying instead of returning
      - **Open fails N times then succeeds**: status becomes `connected: true` and the
        backoff resets to its initial value
      - **Device disappears mid-read**: the read returns, status goes back to
        `connected: false`, and the loop resumes retrying rather than exiting
      - **Grab failure is non-fatal**: an `Open` reporting a failed grab still yields a
        working reader with `grabbed: false`
    - Add a test asserting `nextBackoff` doubles and saturates at `maxBackoff`, pinning the
      documented 30-second worst case
    - **EXPECTED OUTCOME**: all tests PASS on current code (this is preservation, not a bug
      reproduction — nothing here should fail)
    - _Requirements: 2.3, 3.1, 3.2_

  - [ ] 1.3 Add a health-endpoint integration test for the absent-device case
    - Assert `GET /health` returns success and reports `scanner.connected: false` together
      with the configured device path, when that path does not exist
    - Use `handlerTestCase` / `runHandlerTests` per `AGENTS.md`; follow
      `internal/server/handler_scan_headless_test.go`, which already builds a `ScanListener`
      with a `DevicePath` and asserts a `devicePath` field in the health payload
    - Assert against literal expected values, not on mocks returning nothing
    - **EXPECTED OUTCOME**: test PASSES on current code, proving requirement 2.2 already holds
      at the application layer and is now guarded
    - _Requirements: 2.2, 3.6_

- [ ] 2. Fix: remove the create-time device requirement

  - [ ] 2.1 Replace the device declaration with a directory mount in `deploy/docker-compose.yml`
    - DELETE the `devices:` key and its `- /dev/pantry-scanner:/dev/pantry-scanner` entry
      entirely. This is the change that fixes the defect
    - Add `- /dev/input:/dev/input:ro` as a second entry in the existing `volumes:` list,
      with a comment explaining that a directory bind mount reflects nodes udev creates
      later, and that `:ro` is safe because `openEvdev` opens `unix.O_RDONLY`
      (`internal/scanlistener/device.go` line ~68) and a read-only mount does not block the
      `EVIOCGRAB` ioctl
    - Add `device_cgroup_rules:` with the single rule `'c 13:* r'`, with a comment noting
      that char major 13 is the Linux input class and that this is a cgroup allowance, not a
      grant — file ownership and mode still decide what is openable
    - Change `SCANNER_DEVICE: /dev/pantry-scanner` to
      `SCANNER_DEVICE: ${SCANNER_DEVICE:-/dev/input/pantry-scanner}` so an operator can point
      straight at a concrete `/dev/input/eventN` to isolate udev problems from container problems
    - Leave `restart: unless-stopped`, `group_add:`, `ADDR`, and `DB_PATH` untouched
    - _Bug_Condition: isBugCondition(host, compose) == (compose.devices contains a path absent from host)_
    - _Expected_Behavior: compose.devices is empty, so isBugCondition is unsatisfiable and the container is created regardless of scanner presence_
    - _Requirements: 2.1, 2.2, 3.5, 3.7_

  - [ ] 2.2 Move the stable symlink inside the mounted directory in `deploy/udev/99-pantry-scanner.rules`
    - Change `SYMLINK+="pantry-scanner"` to `SYMLINK+="input/pantry-scanner"` in both
      commented examples. This is a requirement, not a preference: udev writes a relative
      symlink, so `/dev/pantry-scanner -> input/eventN` would neither exist nor resolve inside
      a container that mounts only `/dev/input`
    - Change the USB example to match `ENV{ID_VENDOR_ID}` and `ENV{ID_MODEL_ID}` rather than
      `ATTRS{idVendor}` and `ATTRS{idProduct}`, with a comment explaining that udev's
      `input_id` builtin sets those properties directly on the event device while `ATTRS{}`
      must walk up the parent chain and is order-sensitive. Keep `ATTRS{name}` for Bluetooth
    - Keep `GROUP="65532"` and `MODE="0640"`
    - Rewrite the header comment to stay truthful: state that the symlink is now at
      `/dev/input/pantry-scanner`, that it must live under `/dev/input` for the reason above,
      and that per-device scoping now comes from the node's ownership and mode rather than
      from the container seeing only one node. Keep the existing rationale for not adding the
      service user to the `input` group
    - Add NO systemd integration
    - _Requirements: 3.4_

  - [ ] 2.3 Align the documented and compiled-in defaults
    - `deploy/.env.example`: set `SCANNER_DEVICE=/dev/input/pantry-scanner` and update the
      surrounding comments that name the old path. Add one line documenting the debugging
      escape hatch — pointing `SCANNER_DEVICE` at a concrete `/dev/input/eventN` bypasses the
      udev symlink entirely
    - `cmd/server/main.go`: update the `envOrDefault("SCANNER_DEVICE", ...)` default (line
      ~190) and the doc comment naming it (line ~168)
    - `internal/scanlistener/listener.go`: update the `DevicePath` struct default (line ~65)
    - `internal/server/handler_scan_headless_test.go`: update the two assertions referencing
      the old literal (lines ~19 and ~59). These assert the configured path, so updating the
      literal is correct — do not delete either test
    - No logic changes in this subtask
    - Search the repo for any remaining `pantry-scanner` reference and confirm each is either
      the new `/dev/input/` form or a doc sentence Task 5 will rewrite
    - _Requirements: 2.3_

- [ ] 3. Verify preservation tests still pass
  - **Property 3 and Property 4: Preservation**
  - **IMPORTANT**: Re-run the SAME tests from Task 1 — do NOT write new tests
  - Confirm the reconnect-loop tests, the grab-failure case, the backoff saturation test, and
    the health-endpoint test all still pass with the new default device path
  - Run `go build ./...`, `go test ./...`, and `./scripts/test-coverage.sh`; commit the script
    if the threshold auto-increased, per `AGENTS.md`
  - **EXPECTED OUTCOME**: Tests PASS (confirms no regressions from the path and compose changes)
  - _Requirements: 3.1, 3.2, 3.3, 3.6, 3.7_

- [ ] 4. Rewrite `deploy/setup.sh` as an idempotent iteration harness

  - [ ] 4.1 Establish the dispatch skeleton, file copying, and `.env` reconciliation
    - The existing `deploy/setup.sh` is an **uncommitted, untracked draft**. Read it, then
      replace it wholesale — it is not a patch target. Because it is untracked it will not
      appear in `git diff master`
    - Define `SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"` and read every
      source file relative to it, writing only to `/opt/pantry`. The draft conflates the two,
      copying from the working directory while reading `systemd/` and `udev/` from
      `/opt/pantry` — which is why a fresh install fails
    - Dispatch on `"$1"` to `install`, `rule`, `status`, `logs`, `freeze`, `thaw`, `help`.
      Default to `help` with no arguments; never default to a mutating action. Test booleans
      with `[[ "$VAR" == true ]]`, never by executing a variable as a command
    - Copy the whole deploy tree from `SCRIPT_DIR` to `/opt/pantry`, including `systemd/`,
      `udev/`, and `setup.sh` itself, excluding `.env`
    - Reconcile `.env` instead of overwriting it: create it from `.env.example` only when
      absent; when present, append ONLY keys that exist in `.env.example` and are missing from
      `.env`, never altering a key that already has a value, and print each key added. This is
      the most important behavioral fix — the draft's unconditional
      `cp .env.example .env` resets a pinned `PANTRY_IMAGE_TAG` and the following `up -d` then
      redeploys `latest`
    - Ensure `/dev/input` exists before any container start. If it is absent, create it and
      warn, because Docker would otherwise auto-create the bind-mount source as an ordinary
      directory on the root filesystem that never reflects devtmpfs nodes
    - Make per-step fatality explicit: fatal steps log an error with remediation and `exit 1`;
      advisory steps warn and continue. Do not rely on a bare `return 1` propagating through
      `set -euo pipefail`, which aborts the run silently
    - Guard color output with a TTY check, and stop printing a colored prefix for blank lines
    - Delete rather than carry forward: the hand-rolled apt-repository path (including its
      broken escaped-`$()` sources line and its non-idempotent `gpg --dearmor`), `configure_env`,
      `configure_scanner`, the `command -v docker compose` check, and the unused `installed`
      variable
    - _Preservation: an existing /opt/pantry/.env is byte-identical before and after a second install run, except for appended missing keys_
    - _Requirements: 2.6, 3.8_

  - [ ] 4.2 Implement udev rule generation
    - Generate the rule from a detected device; do not ask the operator to hand-edit a template
    - Detect candidates from `/proc/bus/input/devices` and, when `usbutils` is present, `lsusb`.
      Do not match on the literal word "scanner" alone — many scanners present as generic HID
      keyboards. With multiple candidates, show a numbered picker when interactive and fail
      with the candidate list when not; never guess
    - Emit `/etc/udev/rules.d/99-pantry-scanner.rules` with one active rule of the form
      `SUBSYSTEM=="input", KERNEL=="event*", ENV{ID_VENDOR_ID}=="<vid>", ENV{ID_MODEL_ID}=="<pid>", SYMLINK+="input/pantry-scanner", GROUP="65532", MODE="0640"`,
      using `ATTRS{name}` instead for a Bluetooth device
    - Write the file only when its content differs from what is installed, so re-runs are quiet
    - Then `udevadm control --reload` and `udevadm trigger --subsystem-match=input --action=add`.
      Scope the trigger — a bare `udevadm trigger --action=add` re-fires `add` for every device
      on the host
    - Add no `TAG+="systemd"`, `ENV{SYSTEMD_WANTS}`, or `ENV{SYSTEMD_ALIAS}`. Under this design
      a scanner appearing needs no container action at all
    - `rule` runs only this generation plus the reload, trigger, and a status report
    - _Requirements: 2.7, 3.4_

  - [ ] 4.3 Implement the `status` diagnostic
    - Print one line per link with PASS / FAIL / SKIP and, on failure, the exact remediation
      command. The chain, in order: (1) `/opt/pantry` deployment files present; (2) `.env`
      present, echoing effective `PANTRY_IMAGE_TAG`, `HOST_PORT`, `SCANNER_DEVICE`;
      (3) `/dev/input` exists and is on devtmpfs; (4) the udev rule file exists and holds at
      least one non-comment rule; (5) `SCANNER_DEVICE` resolves — `readlink -f` it and report
      the concrete event node; (6) that node's group is 65532 and mode is 0640; (7) the
      container is running per `docker compose ps`; (8) `GET /health` on the effective
      `HOST_PORT` returns `status: ok`; (9) the health payload's scanner object — `connected`,
      `grabbed`, `lastError`, `unmappedKeys`; (10) whether `pantry-update.timer` is active,
      masked, or absent
    - Treat an absent scanner as PASS-with-note, not failure: it is a fully supported state in
      which the container must still run and `/health` must still return ok. Make that visibly
      different from the case operators actually care about — a scanner that is PRESENT but
      unreadable, where the node exists yet health reports `connected: false`
    - Read `HOST_PORT` from the reconciled `.env` and use it for every health request and every
      printed URL. The draft hardcodes 8080 in both places, so any operator who changed the
      port gets a spurious warning and a URL that does not work
    - _Requirements: 2.7_

  - [ ] 4.4 Implement the `install` flow, health polling, and the remaining subcommands
    - `install` converges in this order: verify root via `EUID`; ensure Docker and the compose
      plugin exist, installing with `curl -fsSL https://get.docker.com | sh` if absent, since
      that is what the README specifies and it handles Raspberry Pi OS; add
      `${SUDO_USER:-$USER}` to the docker group, never bare `$USER`, which is root or empty
      under sudo; copy the tree (4.1); reconcile `.env` (4.1); ensure `/dev/input` (4.1);
      install the udev rule (4.2); bring the container up; poll health; print `status`
    - Replace the draft's fixed `sleep 3` with polling against a deadline of at least 60
      seconds, since a first-run image pull on a Pi routinely exceeds three seconds. On timeout,
      fail loudly and dump `docker compose logs --tail=50 pantry`. Never print a success banner
      after a failed verification
    - Do NOT enable `pantry-update.timer` by default; add an explicit `--with-updates` flag for
      when iteration is finished
    - `freeze` masks and `thaw` unmasks `pantry-update.timer`. Explain in `help` that the timer
      fires every five minutes, pulls `latest`, and recreates the container, so leaving it
      enabled while iterating mutates the target mid-experiment
    - `logs` shows `docker compose -f /opt/pantry/docker-compose.yml logs --tail=100 -f pantry`.
      Note in `help` that the image is distroless, so `docker exec` into a shell is unavailable
      and container logs plus `/health` are the only runtime introspection
    - Verify with `bash -n deploy/setup.sh`. `shellcheck` is NOT installed in this sandbox;
      attempt `apt-get install -y shellcheck` and run it if that succeeds, otherwise record
      that it was unavailable and the script was reviewed by reading. Do not claim a clean
      shellcheck run that did not happen
    - _Requirements: 2.1, 2.6, 2.7, 3.8_

  - [ ] 4.5 Add a shellcheck step to `.github/workflows/ci.yml`
    - Several defects in the draft — the escaped-`$()` sources line, bare `$USER`, the
      one-operand `command -v`, the unused variable — are all mechanically detectable
    - _Requirements: 2.6_

- [ ] 5. Update the deployment documentation

  - [ ] 5.1 Rewrite the affected sections of `deploy/README.md`
    - Remove the ordering constraint wherever it appears. The Headless Scanner Input section
      currently states that the udev rule must be installed and the device node must exist
      **before** `docker compose up`, because Docker refuses to start a container whose
      declared device path does not exist. That constraint no longer exists and must not be
      left to mislead a reader. Replace it with the new guarantee: the container starts whether
      or not the scanner is attached, and the scanner may be connected or disconnected at any time
    - Replace the manual Quick Start steps with `sudo ./setup.sh install` followed by
      `sudo ./setup.sh status`, keeping a short prose description so a reader need not read
      shell to understand the deployment
    - Add a subsection documenting the hot-plug model: the container bind-mounts the
      `/dev/input` directory rather than one node; the app retries with backoff from 250ms to
      a 30s cap, so a newly attached scanner is picked up within 30 seconds; and the web UI is
      served throughout, independent of scanner state
    - Update every `/dev/pantry-scanner` reference to `/dev/input/pantry-scanner`, including
      troubleshooting commands
    - Replace the now-obsolete `docker inspect pantry | grep -A 5 Devices` guidance — there is
      no `Devices` array anymore — with a check of the `Mounts` array for the `/dev/input` bind
      mount, and point at `sudo ./setup.sh status` first
    - Fix the pre-existing wrong path `sudo cp pantry/udev/99-pantry-scanner.rules ...`, which
      names a path that does not exist on the Pi; it is `/opt/pantry/udev/...`. Present
      `sudo ./setup.sh rule` as the primary method and keep the manual path as a fallback
    - Fix the pre-existing broken numbering in the Headless Scanner Input Prerequisites
      section, which runs 1, 2, 3, 4 and then restarts at 2, 3, 4, 5
    - Update the security rationale to say that per-device scoping comes from node ownership
      and mode, not from the container seeing only one node
    - Reword the Automatic Updates bullet claiming container recreation drops the scanner
      session — it is dropped but the listener reconnects on its own within 30 seconds. Add
      that `sudo ./setup.sh freeze` masks the update timer while iterating and `thaw` restores it
    - Note that the image is distroless, so `docker exec pantry sh` does not work
    - Restructure the scanner troubleshooting around the distinction that now matters: scanner
      absent (supported — container running, health ok, UI usable) versus scanner present but
      unreadable, listing the three causes for the latter in the order worth checking
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.7, 3.4_

  - [ ] 5.2 Apply the same corrections to `deploy/scan-input.md`
    - New device path; no pre-start ordering requirement; no systemd units; the reconnect
      guarantee
    - Fix the pre-existing inconsistency where two rule examples use `GROUP="pantry"` while the
      numeric GID 65532 is used everywhere else
    - _Requirements: 2.3, 3.4_

- [ ] 6. Write the hardware acceptance runbook
  - Create `deploy/ACCEPTANCE.md`, opening with the single question the task exists to answer:
    does the Pi boot with no scanner attached and serve a usable web UI, and does it start
    reading the scanner whenever the scanner appears
  - Document checks T0-T10, each with an exact command and exact expected result: T0 compose
    schema validation on the Pi via `sudo docker compose config`, closing the sandbox gap;
    **T1 boot with the scanner unplugged — the headline acceptance test**, recording the full
    health payload; T2 hot-plug pickup within 30 seconds plus an end-to-end scan, recording
    actual elapsed time; T3 hot-unplug with `sudo docker inspect -f '{{.State.StartedAt}}' pantry`
    captured before and after, proving the container was NOT restarted or recreated; T4 replug
    recovery with `StartedAt` still unchanged across the whole cycle; T5 boot with the scanner
    attached, confirming the backoff loop absorbs udev racing the container at boot; T6 clean
    install from scratch; T7 a pinned `PANTRY_IMAGE_TAG` surviving a second `install` and still
    being the running image; T8 host-side `ls -l /dev/input/event*` evidence that only the
    scanner's node is group 65532 mode 0640 while others stay `root:input` 0600; T9
    `udevadm verify` (or `udevadm test` on the resolved syspath) and `shellcheck deploy/setup.sh`;
    T10 the `freeze` / `thaw` controls and confirmation that `install` without `--with-updates`
    left the timer disabled
  - Include the contingency section from design "Hardware Acceptance Checking": the
    `ENOENT` / `EACCES` / `EPERM` table distinguishing the three look-alike failures and their
    different fixes; that `EPERM` is the only observation that would invalidate this design;
    that the response to `EPERM` is to re-test with `'c 13:* rwm'` and record the result; and
    that falling back to `privileged: true` or reintroducing a `devices:` entry is prohibited
    because the latter restores the original defect
  - Add a results table with a row per check (T0-T10) and columns for result, date, and notes,
    every result cell left blank for the operator to fill in. Pre-fill nothing
  - This task produces documentation only; it does not execute any check
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 3.4, 3.5, 3.7, 3.8_

- [ ] 7. Checkpoint - Ensure the suite passes, coverage is enforced, and hardware work is handed off
  - Run `go build ./...` and `go test ./...` and confirm every test from Tasks 1 and 3 passes
  - Run `./scripts/test-coverage.sh` to enforce and lock in coverage; commit the updated script
    if the threshold increased, per `AGENTS.md`
  - Confirm the guardrails hold: no `devices:` key in the compose file; no `SYSTEMD_WANTS`,
    `SYSTEMD_ALIAS`, `TAG+=`, or `pantry.service` anywhere under `deploy/`; no file under
    `frontend/` modified; no test or `log.Printf` statement deleted
  - State plainly which verifications were DEFERRED to hardware and were therefore NOT
    performed: compose schema validation, udev rule matching, `shellcheck` if it could not be
    installed, and every scanner behavior. Do not describe the fix as verified
  - Hand off `deploy/ACCEPTANCE.md` to the operator, noting that check T1 is the definition of
    done for this spec
  - Ask the user if any questions or ambiguities arise

---

## Task Dependency Graph

```
Task 1 (Preservation tests — must PASS on current code)
   1.1 Baseline + reconnect contract + coverage inventory
        ▼
   1.2 Reconnect-loop tests          1.3 Health-endpoint test
        └──────────┬───────────────────────┘
                   ▼
Task 2 (Fix — three independent files, no overlap)
   2.1 docker-compose.yml   2.2 udev rule   2.3 .env.example + compiled defaults
        └──────────┬───────────────┬────────────────┘
                   ▼
Task 3 (Re-run Task 1 tests → still PASS; suite + coverage)
                   ▼
Task 4 (setup.sh rewrite — all subtasks edit the SAME file, so strictly sequential)
   4.1 Dispatch + tree copy + .env reconcile
        ▼
   4.2 udev rule generation
        ▼
   4.3 status diagnostic
        ▼
   4.4 install flow + health polling + freeze/thaw/logs
        ▼
   4.5 CI shellcheck step (.github/workflows/ci.yml — independent file)
                   ▼
Task 5 (Docs — independent files)
   5.1 deploy/README.md     5.2 deploy/scan-input.md
                   ▼
Task 6 (deploy/ACCEPTANCE.md — hardware runbook, documentation only)
                   ▼
Task 7 (Checkpoint: suite + coverage + guardrail audit + handoff)
```

```json
{"waves": [["1.1"], ["1.2", "1.3"], ["2.1", "2.2", "2.3"], ["3"], ["4.1"], ["4.2"], ["4.3"], ["4.4"], ["4.5", "5.1", "5.2"], ["6"], ["7"]]}
```

## Notes

- Task 1 is preservation, not bug reproduction. Unlike a typical bugfix spec there is no
  "must FAIL on unfixed code" task, because the bug condition is a Docker create-time check
  against host filesystem state and is unreachable from Go. If a Task 1 test fails, that is a
  real problem to investigate, not the expected red of a reproduction test.
- Task 2's three subtasks touch `deploy/docker-compose.yml`, `deploy/udev/99-pantry-scanner.rules`,
  and (`deploy/.env.example` + two `.go` files + one `_test.go` file) respectively. They do not
  overlap and can share a wave.
- Task 4's subtasks all edit `deploy/setup.sh` and therefore must NOT share a wave, even though
  they are conceptually separable. 4.5 edits `.github/workflows/ci.yml` and is safe to batch
  with Task 5's documentation files.
- The sandbox cannot validate the compose file (no compose plugin — `docker compose config`
  fails with "looking up compose provider failed"), cannot validate udev rules (no `udevadm`),
  cannot lint shell unless `shellcheck` installs, and cannot run frontend tests (no `npm`).
  Every one of those gaps is recorded in `deploy/ACCEPTANCE.md` rather than papered over.
- The container image is distroless. Do not write any verification step that shells into the
  container; `docker exec pantry sh` does not exist. Use `docker compose logs pantry`,
  `GET /health`, and `sudo ./setup.sh status`.
- Definition of done for the whole spec is check T1 in `deploy/ACCEPTANCE.md` passing on real
  hardware: the Pi boots with no scanner attached, the container runs, `/health` returns ok
  with `scanner.connected: false`, the UI loads from another device on the network, and
  plugging the scanner in then makes `scanner.connected` true within 30 seconds with no
  manual intervention.
