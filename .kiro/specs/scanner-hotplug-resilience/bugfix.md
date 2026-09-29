# Bugfix Requirements Document

## Introduction

Pantry is meant to run headlessly on a Raspberry Pi with no monitor, keyboard, or mouse attached. It does not. When the Pi boots without the barcode scanner connected, the container never starts, so nothing is reachable — not the inventory, not the shopping list, not the scan queue — even though none of those features need a scanner.

The cause is a single line in `deploy/docker-compose.yml`. The service declares `devices: - /dev/pantry-scanner:/dev/pantry-scanner`, and Docker refuses to *create* a container whose declared device path is absent. Since `/dev/pantry-scanner` is a udev symlink that only exists while the scanner is plugged in, a boot with the scanner unplugged, asleep, or not yet enumerated fails outright. The operator must notice the failure, attach the scanner, and rerun `docker compose up -d` by hand, which is exactly the monitor-and-keyboard intervention the deployment is supposed to avoid.

The application layer is not the problem. `ScanListener.runDevice` in `internal/scanlistener/listener.go` already retries `Open` indefinitely with exponential backoff, and `cmd/server/main.go` starts the listener in a goroutine before `http.ListenAndServe`, so a missing device already degrades gracefully: HTTP serves, `/health` reports `scanner.connected: false`, and a device that appears later is picked up on the next retry. The defect is entirely in how the container is granted access to the device — a create-time requirement for a hot-pluggable resource.

This spec removes that requirement. The container binds the `/dev/input` directory rather than a single node, so nodes udev creates later become visible without recreating the container, and the existing reconnect loop does the rest.

## Bug Analysis

### Current Behavior (Defect)

1.1 WHEN the Raspberry Pi boots and no barcode scanner is connected THEN the system fails to create the Pantry container, because `deploy/docker-compose.yml` declares a `devices:` entry for `/dev/pantry-scanner` and Docker refuses to create a container whose declared device path does not exist

1.2 WHEN container creation fails because the scanner is absent THEN the system serves no web UI at all, so inventory, shopping list, and scan queue are unreachable even though none of those features require a scanner

1.3 WHEN the scanner is connected after a failed start THEN the system SHALL CONTINUE TO stay down until an operator manually runs `docker compose up -d`, rather than recovering on its own

1.4 WHEN the scanner is disconnected and the container is subsequently recreated for any reason (image update, host reboot, `docker compose up -d`) THEN the system SHALL CONTINUE TO fail to start, so an automatic update that lands while the scanner is unplugged takes the deployment down until the next manual intervention

### Expected Behavior (Correct)

2.1 WHEN the Pi boots and no barcode scanner is connected THEN the system SHALL start the Pantry container and serve the web UI

2.2 WHEN no barcode scanner is connected THEN `GET /health` SHALL return `status: ok` with `scanner.connected: false`, reporting the scanner's absence as a supported state rather than as a service failure

2.3 WHEN a barcode scanner is connected while the container is already running THEN the system SHALL begin reading scans from it within 30 seconds without any operator action

2.4 WHEN a barcode scanner is disconnected while the container is running THEN the system SHALL continue serving the web UI, and SHALL NOT stop, restart, or recreate the container

2.5 WHEN a barcode scanner is reconnected after a disconnection THEN the system SHALL resume reading scans from it within 30 seconds without any operator action

2.6 WHEN the deployment setup script is run more than once on the same host THEN the system SHALL converge to the same working state without overwriting any configuration value already present in `/opt/pantry/.env`

2.7 WHEN an operator needs to diagnose a scanner problem THEN the setup script SHALL report, for each link in the chain from udev rule to scan ingestion, whether that link is working and what to run if it is not

### Unchanged Behavior (Regression Prevention)

3.1 WHEN a barcode scanner is connected and readable THEN the system SHALL CONTINUE TO read barcodes from its evdev node and enqueue scan entries exactly as it does today

3.2 WHEN the scanner's evdev node is opened THEN the system SHALL CONTINUE TO request an exclusive grab so scanner keystrokes do not also reach the console, and SHALL CONTINUE TO keep capturing scans when that grab fails

3.3 WHEN the `STOCK_IN` or `STOCK_OUT` control barcode is scanned THEN the system SHALL CONTINUE TO switch scanner mode rather than enqueue a product scan

3.4 WHEN input devices other than the designated scanner are attached to the host THEN the container SHALL CONTINUE TO have no read access to them, so Pantry does not become a keylogger for every attached keyboard

3.5 WHEN the container runs THEN it SHALL CONTINUE TO run as uid 65532 and SHALL CONTINUE TO run unprivileged

3.6 WHEN `SCAN_INPUT=stdin` is set for local development THEN the system SHALL CONTINUE TO read scans from standard input and SHALL CONTINUE TO require no scanner device

3.7 WHEN the container image is updated or the container is recreated THEN the `pantry-data` named volume SHALL CONTINUE TO preserve the SQLite database across the change

3.8 WHEN `PANTRY_IMAGE_TAG` in `/opt/pantry/.env` is pinned to a specific commit SHA THEN the system SHALL CONTINUE TO deploy that pinned image, and SHALL NOT be silently reverted to `latest` by any setup or update action
