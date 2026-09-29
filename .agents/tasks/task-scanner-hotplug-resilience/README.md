# Task: scanner hot-plug resilience (Design B)

## Goal

Pantry must boot and serve its web UI on a headless Raspberry Pi **whether or not the
barcode scanner is connected**, and must pick up the scanner whenever it appears or
reappears — without any systemd device-unit machinery.

## Why this shape

`docker-compose.yml` currently declares:

```yaml
devices:
  - /dev/pantry-scanner:/dev/pantry-scanner
```

Docker refuses to **create** a container whose declared device path is absent, so a
boot with no scanner attached fails. That single line is the entire boot failure.

The Go application already tolerates a missing device. `internal/scanlistener/listener.go`
`runDevice()` retries `Open` forever with backoff (250ms, doubling, capped at 30s), and
`cmd/server/main.go` starts it with `go listener.Run(...)` on line 121 *before*
`http.ListenAndServe` on line 125. So HTTP already serves fine with no scanner, `/health`
already reports `scanner.connected: false`, and a scanner plugged in later is already
picked up within 30 seconds worst case.

So the fix is to stop making the device a container-creation precondition, and instead
give the container standing permission to read input devices as they come and go.

## Design B in one paragraph

Remove the `devices:` binding. Bind-mount the `/dev/input` **directory** instead, so
device nodes created later by udev become visible inside the container. Add a device
cgroup rule permitting reads of char-major-13 (input class). Move the udev `SYMLINK`
to `input/pantry-scanner` so the stable name lives *inside* the bind-mounted directory
and its relative target resolves. Keep `restart: unless-stopped` so the container starts
at boot. Access stays scoped to the one scanner by **file permissions** on the node
(`GROUP="65532"`, `MODE="0640"`), exactly as before.

## Explicitly rejected alternative

The `feature/udev-driven-container-lifecycle` branch instead gates container start on a
udev-driven systemd unit. Do **not** build on that branch or reuse its approach. It
couples the web UI's availability to scanner presence (scanner sleeps -> UI goes dark),
and it is broken as written: it binds to `dev-pantry\x2dscanner.device`, a unit that is
never created because `SYMLINK+=` does not produce a device-unit alias (that requires
`ENV{SYSTEMD_ALIAS}`, which the branch never sets).

## Hard guardrails

- Work on **`feature/scanner-hotplug-resilience`**, which already exists, already contains
  these plan files, and is based on `master`. Do not create a new branch.
- Never commit directly to `master`.
- Do **not** branch from or cherry-pick from `feature/udev-driven-container-lifecycle`.
- Do **not** add `pantry.service`, `TAG+="systemd"`, `ENV{SYSTEMD_WANTS}`, or
  `ENV{SYSTEMD_ALIAS}` anywhere. Design B needs no systemd integration.
- Do **not** delete existing log lines or existing tests. If a change appears to require
  deleting a test, stop and record it in `findings` instead.
- Do not change `restart: unless-stopped`.
- Do not touch frontend code. This task has no frontend component, and `npm` is not
  available in the sandbox.

## Note on the existing `deploy/setup.sh`

There is an **uncommitted, untracked** draft at `deploy/setup.sh` in the working tree. It
is not on any branch. Treat it as a discard-and-rewrite input for FEAT-003, not as
working code — it overwrites `/opt/pantry/.env` on every run, never copies the `systemd/`
or `udev/` subdirectories it later reads from, and writes a literal unexpanded
`$(dpkg --print-architecture)` into an apt sources file. Because it is untracked it will
not appear in `git diff master`, so do not expect it in diff-based verification.

## Sandbox tooling limits (verified)

| Tool | Available | Consequence |
|---|---|---|
| `go` | yes | run `go build ./...`, `go test ./...`, `./scripts/test-coverage.sh` |
| `docker` CLI | yes, but **no compose plugin** | `docker compose config` cannot validate the compose file locally |
| `shellcheck` | no | review shell changes by reading; `bash -n` still works |
| `udevadm` | no | udev rule syntax cannot be validated locally |
| `npm` / `node` | no | frontend tests cannot run — do not touch frontend |
| `python3` | yes, **no pyyaml** | no YAML parse check unless `pip install pyyaml` succeeds |

Anything that cannot be verified in the sandbox belongs in FEAT-005's hardware runbook.
Do not claim it verified.

## The container is distroless

`docker exec pantry sh` will not work — there is no shell in the image. All runtime
verification goes through `GET /health` and `docker logs pantry`. Do not write
verification steps that shell into the container.

## Features

| ID | Description |
|---|---|
| FEAT-001 | Baseline + Go regression tests pinning missing-device tolerance |
| FEAT-002 | Container device-access model change (the actual fix) |
| FEAT-003 | Rewrite `deploy/setup.sh` as an idempotent subcommand script with a `status` doctor |
| FEAT-004 | Documentation update for the new model |
| FEAT-005 | Hardware acceptance runbook, to be executed by the human on the Pi |

Work them in order. FEAT-002 depends on FEAT-001's baseline, FEAT-003 depends on
FEAT-002's final paths, and FEAT-004 documents the CLI surface FEAT-003 creates.

## Definition of done

The task is complete when FEAT-005's T1 passes on real hardware: the Pi boots with no
scanner attached, the container is running, `/health` returns ok with
`scanner.connected: false`, and the web UI loads from another device on the network —
and then plugging the scanner in makes `scanner.connected` true within 30 seconds with
no manual intervention. Everything before T1 is preparation; nothing else constitutes
proof.
