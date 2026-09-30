# Pantry Scanner Hot-Plug Resilience — Hardware Acceptance Runbook

**The one question this runbook exists to answer:** Does the Pi boot with no scanner attached and serve a usable web UI, and does it start reading the scanner whenever the scanner appears?

Everything below verifies that guarantee and the behaviors around it. These checks **cannot** be run in the CI sandbox — there is no compose plugin, no `udevadm`, no real devtmpfs, and no scanner. They must be executed by an operator on the target Raspberry Pi. **No check here may be reported as passed from the sandbox.**

The image is **distroless**, so `docker exec pantry sh` does not work. All runtime introspection is via `docker compose logs pantry`, `GET /health`, and `sudo ./setup.sh status`.

**Definition of done for this spec: check T1 passes on real hardware.**

---

## Preconditions

- A Raspberry Pi running 64-bit Raspberry Pi OS (arm64).
- The `deploy/` directory copied onto the Pi.
- A USB or Bluetooth barcode scanner for the checks that require one.
- Network access from another device to reach `http://<pi-ip>:<HOST_PORT>`.

Throughout, `HOST_PORT` is whatever is set in `/opt/pantry/.env` (default `8080`). Read it once:

```bash
grep '^HOST_PORT=' /opt/pantry/.env
```

---

## Checks

### T0 — Compose schema validates on the Pi

Closes the sandbox gap: the compose file could not be validated in CI because no compose plugin exists there.

```bash
cd /opt/pantry
sudo docker compose config
```

**Expected:** The command prints the fully resolved configuration and exits 0. The output contains a `/dev/input:/dev/input` bind mount and a `device_cgroup_rules` entry `c 13:* r`, and contains **no** `devices:` key.

---

### T1 — Boot with the scanner unplugged (THE HEADLINE ACCEPTANCE TEST)

This is the definition of done.

1. Ensure the scanner is physically disconnected.
2. Reboot the Pi: `sudo reboot`.
3. After boot, from another device on the network, open `http://<pi-ip>:<HOST_PORT>` and confirm the UI loads.
4. On the Pi:
   ```bash
   curl -s http://localhost:<HOST_PORT>/health
   ```

**Expected:** The container is running, the UI loads from another device, and `/health` returns `status: ok` with `scanner.connected: false`. Record the **full health payload** here:

```
(paste full /health JSON)
```

---

### T2 — Hot-plug pickup within 30 seconds, plus an end-to-end scan

With the container running (from T1), plug in the scanner and start a timer.

```bash
# Poll until connected, noting the time
while true; do
  curl -s http://localhost:<HOST_PORT>/health | grep -o '"connected":[a-z]*'
  sleep 2
done
```

Then scan a product barcode and confirm it reaches the queue (check the UI's scan queue or `GET /api/scans`).

**Expected:** `scanner.connected` becomes `true` within 30 seconds of plugging in, with no operator action beyond attaching the device. A subsequent scan is enqueued end-to-end. **Record actual elapsed time to connect:**

```
Elapsed time to connect: ______ seconds
```

---

### T3 — Hot-unplug leaves the UI up and does NOT recreate the container

Capture the container start time, unplug the scanner, and capture it again.

```bash
sudo docker inspect -f '{{.State.StartedAt}}' pantry   # BEFORE
# ... unplug the scanner, wait ~10s ...
curl -s http://localhost:<HOST_PORT>/health            # UI still ok, connected:false
sudo docker inspect -f '{{.State.StartedAt}}' pantry   # AFTER
```

**Expected:** The UI stays up, `/health` returns `status: ok` with `scanner.connected: false`, and `StartedAt` is **identical** before and after — proving the container was not stopped, restarted, or recreated.

```
StartedAt BEFORE: ______________________
StartedAt AFTER:  ______________________
```

---

### T4 — Replug recovery, container still not recreated

Re-attach the scanner after T3.

```bash
sudo docker inspect -f '{{.State.StartedAt}}' pantry   # should match T3
# ... replug, wait for reconnect ...
curl -s http://localhost:<HOST_PORT>/health            # connected:true again
sudo docker inspect -f '{{.State.StartedAt}}' pantry   # STILL unchanged
```

**Expected:** `scanner.connected` returns to `true` within 30 seconds, and `StartedAt` is unchanged across the entire connect/disconnect/reconnect cycle (T2→T3→T4).

```
Elapsed time to reconnect: ______ seconds
StartedAt unchanged across cycle? (Y/N): ______
```

---

### T5 — Boot with the scanner attached

Confirms the backoff loop absorbs udev racing the container at boot.

1. Attach the scanner.
2. `sudo reboot`.
3. After boot: `curl -s http://localhost:<HOST_PORT>/health`.

**Expected:** Within 30 seconds of boot, `scanner.connected: true`. Even if the container starts before udev has created the symlink, the reconnect loop resolves it without a restart.

---

### T6 — Clean install from scratch

On a Pi (or after removing `/opt/pantry` and the udev rule):

```bash
cd /path/to/deploy
sudo ./setup.sh install
sudo ./setup.sh status
```

**Expected:** `install` installs Docker (if needed), copies the tree to `/opt/pantry`, creates `.env`, installs the udev rule, starts the container, and polls health to success. `status` reports every link PASS (scanner absent is PASS-with-note).

---

### T7 — A pinned image tag survives a second install

1. Pin a specific commit SHA in `/opt/pantry/.env`:
   ```bash
   sudo sed -i 's/^PANTRY_IMAGE_TAG=.*/PANTRY_IMAGE_TAG=<known-sha>/' /opt/pantry/.env
   cd /opt/pantry && sudo docker compose up -d
   ```
2. Re-run install:
   ```bash
   cd /path/to/deploy && sudo ./setup.sh install
   ```
3. Verify:
   ```bash
   grep '^PANTRY_IMAGE_TAG=' /opt/pantry/.env
   sudo docker inspect -f '{{.Config.Image}}' pantry
   ```

**Expected:** `PANTRY_IMAGE_TAG` is still the pinned SHA (NOT reset to `latest`), and the running image is that SHA.

```
Pinned SHA: ______________________
After second install, tag = ______________________
Running image = ______________________
```

---

### T8 — Only the scanner's node is group-readable; others stay private

With the scanner attached:

```bash
ls -l /dev/input/event*
readlink -f /dev/input/pantry-scanner
```

**Expected:** The scanner's event node (the target of `/dev/input/pantry-scanner`) is `group 65532, mode 0640`. Every other `event*` node stays `root:input` mode `0600`. This is the sole mechanism scoping the container to one device.

```
(paste ls -l /dev/input/event* output)
```

---

### T9 — udev rule and shell lint

```bash
# Verify the rule (choose whichever your udev version supports)
sudo udevadm verify /etc/udev/rules.d/99-pantry-scanner.rules
# or, against the resolved syspath:
sudo udevadm test $(udevadm info -q path -n /dev/input/pantry-scanner)

# Lint the setup script
shellcheck /opt/pantry/setup.sh
```

**Expected:** `udevadm` reports the rule matches the scanner and applies the `SYMLINK`, `GROUP`, and `MODE`. `shellcheck` reports no errors.

> **Sandbox note:** `shellcheck` was not installable in the CI sandbox during development, and `udevadm` is unavailable there. The script was verified with `bash -n` and reviewed by reading; a CI job (`.github/workflows/ci.yml` → `shell`) now runs `shellcheck deploy/setup.sh` on every push.

---

### T10 — Update-timer freeze / thaw controls

```bash
# install without --with-updates should leave the timer disabled
systemctl is-enabled pantry-update.timer || echo "disabled (expected after plain install)"

sudo ./setup.sh freeze
systemctl is-enabled pantry-update.timer   # masked

sudo ./setup.sh thaw
systemctl is-enabled pantry-update.timer   # unmasked/enabled
```

**Expected:** A plain `install` leaves `pantry-update.timer` disabled. `freeze` masks it; `thaw` restores it.

---

## Contingency: distinguishing the three look-alike failures

If T2/T5 do not connect while the scanner IS attached, read `scanner.lastError` from `/health` and match it here. These three look identical in the UI but have different fixes — getting this wrong wastes the most time.

| Error | Meaning | Fix |
|---|---|---|
| `ENOENT` / no such file | udev rule did not match; no symlink created | fix the rule's match keys and re-run `sudo ./setup.sh rule` |
| `EACCES` / permission denied | node's group is not 65532 | fix `GROUP` in the rule or `SCANNER_GID` in `.env`, reload udev |
| `EPERM` / operation not permitted | device cgroup denied the open | `device_cgroup_rules` is not taking effect |

**`EPERM` is the only observation that would invalidate this design.** If it occurs:

1. Re-test with the rule widened to `'c 13:* rwm'` in `docker-compose.yml`'s `device_cgroup_rules` to determine whether the mode string is at fault, and **record the result**.
2. Do **NOT** fall back to `privileged: true`.
3. Do **NOT** reintroduce a `devices:` entry — that restores the original boot-failure defect this spec fixes.
4. If the cgroup approach genuinely cannot work on the target kernel, **stop and escalate with the evidence** rather than working around it.

---

## Results

Fill in each row as you run the check. Leave everything blank until performed — do not pre-fill.

| Check | Description | Result (PASS/FAIL) | Date | Notes |
|-------|-------------|--------------------|------|-------|
| T0 | Compose schema validates on the Pi | | | |
| T1 | Boot with scanner unplugged (headline) | | | |
| T2 | Hot-plug pickup < 30s + end-to-end scan | | | |
| T3 | Hot-unplug: UI up, container not recreated | | | |
| T4 | Replug recovery, StartedAt unchanged | | | |
| T5 | Boot with scanner attached | | | |
| T6 | Clean install from scratch | | | |
| T7 | Pinned image tag survives second install | | | |
| T8 | Only scanner node group-readable | | | |
| T9 | udev rule verify + shellcheck | | | |
| T10 | freeze/thaw + timer disabled by default | | | |
