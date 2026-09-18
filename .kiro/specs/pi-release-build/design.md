# Design Document

## Overview

Pantry becomes a single artifact: one Go binary with the compiled React app embedded, packaged as a `linux/arm64` container image published to `ghcr.io/rhionin/pantry` on every push to `main`. A Raspberry Pi operator runs it with `docker compose up -d` and updates it with `docker compose pull && docker compose up -d`, or opts into unattended updates by installing the shipped systemd service and timer, which run that same pull-and-restart on an interval.

Five pieces of work, largely independent:

| Area | Artifact | Purpose |
|---|---|---|
| Asset serving | `internal/webui` | Embed the built SPA, serve it with fallback |
| Route composition | `internal/server/server.go` | Mount the web UI without disturbing `/api` and `/health` |
| Packaging | `Dockerfile` | node → go → distroless, arm64 output |
| Automation | `.github/workflows/{ci,release}.yml` | Validate every change; publish on `main` |
| Deployment | `deploy/{docker-compose.yml,.env.example,systemd/,README.md}` | Pi-side run and update procedure, plus opt-in systemd auto-update units |

No API behavior, data model, or frontend feature changes. The only runtime change visible to existing API clients is that previously-404 non-`/api` paths now return the SPA document.

## Architecture

```
Request ──▶ root ServeMux
              ├── "/api", "/api/"       ──▶ apiMux (all existing routes)
              ├── "/health", "/health/" ──▶ apiMux
              └── "/"                   ──▶ webui.Handler ──▶ embed.FS
                                                             └── fallback: index.html
```

`NewHandler` keeps building today's mux verbatim (now a local named `apiMux`), then wraps it in a `root` mux that delegates the two API prefixes back to `apiMux` and gives everything else to the web UI handler. The signature does not change, so `cmd/server/main.go` and the `exchanges()` test helper need no edits.

### Build and publish flow

```
push to main
   └── Release_Workflow
         ├── validate  (reusable call into CI_Workflow)
         └── publish
               buildx ──▶ Dockerfile
                            ├── frontend stage  (BUILDPLATFORM, amd64)  npm ci && npm run build
                            ├── build stage     (BUILDPLATFORM, amd64)  COPY dist → internal/webui/assets
                            │                                           GOARCH=arm64 go build
                            └── runtime stage   (linux/arm64)           distroless static:nonroot
                       └──▶ ghcr.io/rhionin/pantry:{latest,main,<sha>}
```

### Update flow on the Pi

```
pantry-update.timer  (OnBootSec, then OnUnitActiveSec=5min)
   └── pantry-update.service  (Type=oneshot, root)
         ├── ExecStart: docker compose -f /opt/pantry/docker-compose.yml pull
         └── ExecStart: docker compose -f /opt/pantry/docker-compose.yml up -d
                          ├── digest unchanged ──▶ no-op
                          └── digest changed   ──▶ recreate pantry, reattach pantry-data
```

## Components and Interfaces

### 1. `internal/webui`

Layout:

```
internal/webui/
  webui.go                 // embed directive, handler
  webui_test.go
  assets/
    index.html             // tracked Placeholder_Assets
```

Public surface:

```go
// NewHandler serves the frontend assets compiled into the binary.
func NewHandler() http.Handler

// NewHandlerFS serves an arbitrary asset tree. NewHandler is NewHandlerFS over
// the embedded tree; tests use it to inject synthetic asset trees.
func NewHandlerFS(assets fs.FS) http.Handler
```

Embedding:

```go
//go:embed all:assets
var embedded embed.FS
```

`all:` matters — a Vite build can emit `.vite/` metadata and underscore-prefixed chunk names, which the default `//go:embed` pattern silently skips. `NewHandler` calls `fs.Sub(embedded, "assets")` so request paths map directly onto FS names.

Request handling, in order:

1. **Method gate.** Anything other than `GET` or `HEAD` gets `405` with `Allow: GET, HEAD`, before any lookup. This applies even to paths that do exist as assets — the web UI surface is read-only, and gating first keeps the rule easy to state and test.
2. **Name resolution.** `name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")`. `ServeMux` has already cleaned the path, so traversal attempts arrive normalized; anything `fs.Stat` rejects falls through to step 4.
3. **Asset hit.** `fs.Stat` finds a regular file → delegate to `http.FileServerFS(assets)`, which handles `Content-Type` via `mime.TypeByExtension`, range requests, and `HEAD`. `index.html` is special-cased to step 4 to avoid `FileServer`'s `/index.html` → `./` redirect.
4. **SPA fallback.** Empty name, a directory, a stat error, or a miss → `http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))` with status 200. `ServeContent` sets `text/html; charset=utf-8` from the name and handles `HEAD` correctly.

The `index.html` bytes are read once in the constructor and held in memory.

Two details that only bite in the container:

- **MIME table.** Go's built-in table has no `.woff2`/`.woff`/`.ttf`, and distroless has no `/etc/mime.types` to fall back on, so fonts would be served as `application/octet-stream`. A package `init` registers them with `mime.AddExtensionType`.
- **Cache headers.** `index.html` is served with `Cache-Control: no-cache`; anything under `/assets/` (Vite's content-hashed output) gets `Cache-Control: public, max-age=31536000, immutable`. Without the first, a browser holding a cached `index.html` after an image update requests hashed chunks that no longer exist.

Placeholder `assets/index.html` is a minimal document naming itself as the placeholder and pointing at `npm run build`, so a mis-built image is obvious in the browser rather than looking like a blank app.

### 2. Route composition in `internal/server/server.go`

```go
// newAPIMux holds every existing registration, unchanged.
apiMux, scanQueue := newAPIMux(catalog, lookupService, refresher, db)

root := http.NewServeMux()
root.Handle("/api", apiMux)
root.Handle("/api/", apiMux)
root.Handle("/health", apiMux)
root.Handle("/health/", apiMux)
root.Handle("/", webui.NewHandler())

return root, scanQueue
```

Prefix delegation to the existing mux, not one flat mux with a `/` catch-all: `ServeMux` synthesizes `405 Method Not Allowed` only when no pattern matches, so a registered `/` would turn today's `DELETE /api/products/1` → `405 Allow: ...` into an SPA `200`. `apiMux` has no catch-all, so nested behind a prefix its own not-found and method-not-allowed logic still runs. Nested muxes see the unmodified request path (no `StripPrefix`), so wildcard patterns like `PUT /api/products/{id}` and `r.PathValue("id")` keep working.

The registration list moves into an unexported `newAPIMux` rather than staying inline in `NewHandler`. That is what lets Property 6 build a bare `apiMux` alongside the composed root handler from a test in the same package, without a second copy of the route table drifting away from the first.

Resulting behavior:

| Request | Handled by | Status |
|---|---|---|
| `GET /` | webui | 200 `index.html` |
| `GET /assets/index-a1b2.js` | webui | 200, `text/javascript` |
| `GET /inventory` (client route) | webui | 200 `index.html` |
| `POST /inventory` | webui | 405, `Allow: GET, HEAD` |
| `GET /api/products` | apiMux | unchanged |
| `GET /api/nope` | apiMux | 404 (no fallback) |
| `DELETE /api/products/1` | apiMux | 405 (unchanged from today) |
| `GET /api` | apiMux | 404 |
| `GET /health` | apiMux | 200 |
| `POST /health` | apiMux | 405 |
| `GET /health/deep` | apiMux | 404 |

### 3. `Dockerfile`

```dockerfile
# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/frontend/dist/ ./internal/webui/assets/
RUN mkdir -p /data
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/pantry-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/pantry-server /usr/local/bin/pantry-server
COPY --from=build --chown=nonroot:nonroot /data /data
ENV ADDR=":8080" DB_PATH="/data/pantry.db"
EXPOSE 8080
VOLUME ["/data"]
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/pantry-server"]
```

Decisions:

- **`--platform=$BUILDPLATFORM` on both build stages.** The Go toolchain cross-compiles natively, so the runner's amd64 CPU does the work at full speed and `TARGETARCH=arm64` (supplied by buildx from `platforms: linux/arm64`) selects the output. Without this, both stages would run under QEMU emulation and a `npm ci` + Vite build would take minutes instead of seconds. The runtime stage deliberately omits `--platform` so the produced image manifest is arm64.
- **`docker/setup-qemu-action` is kept as insurance, not a requirement.** The runtime stage only `COPY`s, so no arm64 instruction is ever executed; the line matters only if a future runtime stage adds a `RUN`.
- **`CGO_ENABLED=0` is free here** because the SQLite driver is `modernc.org/sqlite` (pure Go). A cgo driver would rule out the static base entirely.
- **`/data` created and chowned in the build stage**, then copied, because distroless has no shell to `RUN mkdir`. A Docker *named* volume inherits ownership from the image path, so `nonroot` (uid 65532) can write the database with no operator action. A host bind mount does not — hence the compose file uses a named volume and the guide documents the `chown 65532:65532` step for operators who prefer a bind mount.
- **No `HEALTHCHECK`.** Distroless ships no shell, `curl`, or `wget`, so an image-level healthcheck cannot be expressed. `/health` verification lives in the deployment guide instead.

### 4. GitHub Actions

**`.github/workflows/ci.yml`** — triggers on `pull_request` targeting `main`, `push` to `main`, and `workflow_call` (so the release workflow can gate on it). `permissions: contents: read`. Two parallel jobs:

- `go`: `actions/setup-go` with `go-version-file: go.mod` (satisfies "Go version from `go.mod`"), then `./scripts/test-coverage.sh`. The script needs `bc`; it is present on GitHub's `ubuntu-latest` image, and the job adds a one-line `apt-get install -y bc` guard rather than depending on that. The script rewrites its own `COVERAGE_THRESHOLD` when coverage rises — in CI that just dirties a file nobody commits, which is harmless and intentional.
- `frontend`: `actions/setup-node` with `node-version-file: frontend/.nvmrc` (new file pinning Node 24, matching `@types/node ^24` and Vite 8), `cache: npm`, then `npm ci`, `npx tsc -b`, `npm run lint`, `npx vitest --run`.

No step suppresses errors (`|| true`, `continue-on-error`, or a pipe that masks the exit status), which is what makes non-zero exits surface as a failed commit status.

**`.github/workflows/release.yml`** — triggers on `push` to `main`. `permissions: contents: read, packages: write`.

```yaml
jobs:
  validate:
    uses: ./.github/workflows/ci.yml
  publish:
    needs: validate
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-qemu-action@v3        # optional; see Dockerfile notes
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@v6
        with:
          context: .
          platforms: linux/arm64
          push: true
          provenance: false
          cache-from: type=gha
          cache-to: type=gha,mode=max
          tags: |
            ghcr.io/rhionin/pantry:latest
            ghcr.io/rhionin/pantry:main
            ghcr.io/rhionin/pantry:${{ github.sha }}
```

Decisions:

- **Reusable-workflow call, gated with `needs: validate`.** One definition of "validated", and a hard gate; a `workflow_run` trigger would fire asynchronously and could publish an unvalidated commit.
- **Literal tag list rather than `docker/metadata-action`.** The requirement is the *full* commit SHA, and `metadata-action`'s `type=sha` emits a `sha-`-prefixed, short-by-default tag.
- **Single build-push step**, so a failure in the frontend build, the Go compile, or the push leaves `latest` pointing at the previous digest. Nothing pushes incrementally.
- **`provenance: false`** keeps the published reference a plain single-platform image rather than an attestation index, which older Docker versions on a Pi handle less predictably.
- **GHA layer cache** keeps a typical release near a minute; the `npm ci` layer is the main beneficiary.

### 5. `deploy/docker-compose.yml`

```yaml
services:
  pantry:
    image: ghcr.io/rhionin/pantry:${PANTRY_IMAGE_TAG:-latest}
    container_name: pantry
    restart: unless-stopped
    stdin_open: true
    tty: true
    ports:
      - "${HOST_PORT:-8080}:8080"
    environment:
      ADDR: ":8080"
      DB_PATH: /data/pantry.db
      PRODUCT_CACHE_TTL: ${PRODUCT_CACHE_TTL:-720h}
      PRODUCT_MISS_TTL: ${PRODUCT_MISS_TTL:-168h}
      DISABLE_EXTERNAL_PRODUCT_LOOKUP: ${DISABLE_EXTERNAL_PRODUCT_LOOKUP:-false}
      STOCK_IN_CONTROL_BARCODE: ${STOCK_IN_CONTROL_BARCODE:-STOCK_IN}
      STOCK_OUT_CONTROL_BARCODE: ${STOCK_OUT_CONTROL_BARCODE:-STOCK_OUT}
      HEADLESS_USER_ID: ${HEADLESS_USER_ID:-user-1}
    volumes:
      - pantry-data:/data

volumes:
  pantry-data:
```

One service, one named volume, no Compose profile (Requirement 7.2). No top-level `version:` key: it is obsolete under Compose v2 and modern Docker warns on it with every command, which is noise on exactly the box where the operator is least able to tell noise from an error.

The defaults mirror `cmd/server/main.go` exactly: `720h` = `defaultProductCacheTTL` (30 days), `168h` = `defaultMissTTL` (7 days), `STOCK_IN`/`STOCK_OUT`/`user-1` from `loadScanListenerConfig`, and `DISABLE_EXTERNAL_PRODUCT_LOOKUP` anything-but-`"true"` meaning enabled. Together with `HOST_PORT` and `PANTRY_IMAGE_TAG`, these are the operator-overridable surface (Requirement 5.6).

`ADDR` and `DB_PATH` are pinned rather than `${...}`-substituted (Requirement 5.7). `ADDR`'s port must agree with the container side of the port mapping, and `DB_PATH` must sit inside the volume mount; making either operator-overridable creates a silent-data-loss footgun. Changing the *host* port goes through `HOST_PORT`, which substitutes only the left half of `"${HOST_PORT:-8080}:8080"` — the container side stays `8080` and therefore stays in agreement with `ADDR` (Requirement 5.8).

`.env` handling: Compose auto-loads `.env` from the compose file's directory. The repo ships `deploy/.env.example` documenting every variable; the operator copies it to `deploy/.env`. `.gitignore` gains `deploy/.env`.

`stdin_open` + `tty` are necessary but not sufficient for the scan listener — they keep the container's stdin open, but a USB HID scanner types into the *Pi's* console, not into the container. Feeding it requires `docker attach pantry` (or piping a device into the container). The guide states this as a known limitation.

The named volume is what makes container replacement safe: `pantry-data` is declared under `volumes:` and referenced by name, so any recreation — manual or automatic — reattaches the same volume and `pantry.db` is untouched (Requirement 7.9). The database must not live in the container's own writable layer.

### 6. `deploy/systemd/` — the Auto_Update_Units

Two files, copied to the Pi by the operator. They are not referenced by the compose file and have no effect until installed and enabled.

```ini
# deploy/systemd/pantry-update.service
[Unit]
Description=Pull and restart the Pantry deployment
Wants=network-online.target docker.service
After=network-online.target docker.service

[Service]
Type=oneshot
TimeoutStartSec=15min
WorkingDirectory=/opt/pantry
ExecStart=/usr/bin/docker compose -f /opt/pantry/docker-compose.yml pull
ExecStart=/usr/bin/docker compose -f /opt/pantry/docker-compose.yml up -d
```

```ini
# deploy/systemd/pantry-update.timer
[Unit]
Description=Check for a new Pantry release on an interval

[Timer]
OnBootSec=2min
OnUnitActiveSec=5min
Unit=pantry-update.service

[Install]
WantedBy=timers.target
```

Decisions:

- **`Type=oneshot` with two `ExecStart=` lines.** systemd runs them in order and treats the unit as finished when the last one exits; a non-zero exit from `pull` aborts the unit before `up -d` runs, which is exactly right — a failed pull must not trigger a restart (Requirement 7.3).
- **`TimeoutStartSec=15min`.** `TimeoutStartSec` covers the whole `ExecStart` sequence of a oneshot unit, and it defaults to 90 seconds. A first pull of the image over a slow home connection on a Pi can exceed that, and the default would SIGTERM the pull mid-transfer and mark the unit failed on every attempt. A finite ceiling rather than `infinity`: a wedged pull that never returns would otherwise block every later run, because the timer will not start a second instance while the first is still active.
- **Absolute `/usr/bin/docker`.** `ExecStart` is not run through a shell and systemd does not resolve `PATH` the way a login shell does, so a bare `docker` fails with a 203/EXEC error on every run. `/usr/bin/docker` is the path installed by Docker's apt repository on Raspberry Pi OS; the guide tells the operator to confirm with `command -v docker` before installing.
- **`WorkingDirectory=/opt/pantry`.** A unit run by systemd has no shell cwd — it starts in `/`. Compose resolves `.env` relative to the compose file's directory, and setting the working directory to that same directory makes both resolutions agree, so the operator's `.env` overrides apply to the automatic path exactly as they do to a hand-typed `docker compose up -d`. Without it, the deployment silently falls back to every default in the compose file.
- **`up -d` is the idempotence mechanism** (Requirements 7.4, 7.5). Compose compares the pulled image against the running container and recreates only when the digest differs; an unchanged tag is a no-op. That is what makes a 5-minute timer safe — nearly every run does nothing at all, so a short interval costs a registry request rather than a container restart.
- **`OnUnitActiveSec=5min` plus `OnBootSec=2min`.** The first sets the interval between attempts; the second means a Pi that has been off for a week checks shortly after boot instead of waiting out a full interval from timer activation (Requirement 7.6).
- **`Persistent=true` is not set.** It only replays missed *calendar* triggers (`OnCalendar=`); with a monotonic `OnUnitActiveSec=` it does nothing. `OnBootSec=` already covers the reboot case, so setting `Persistent=` would be cargo cult.
- **Install path is `/opt/pantry`.** The unit's `ExecStart` and `WorkingDirectory` need real absolute paths, so the guide names one concrete location and puts `docker-compose.yml` and `.env` there. An operator who deploys elsewhere must edit both `ExecStart` lines and `WorkingDirectory` to match.

Installation and lifecycle:

```
sudo cp deploy/systemd/pantry-update.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now pantry-update.timer
```

Until `enable --now` runs, the units are inert — a copied-but-unenabled timer makes no update attempt (Requirement 7.7). Disabling is `sudo systemctl disable --now pantry-update.timer`.

Interval overrides go through a drop-in, `sudo systemctl edit pantry-update.timer`, rather than editing the installed file. A drop-in survives reinstalling the shipped unit and keeps the operator's choice visible as a separate file; note that a drop-in *adds* to `OnUnitActiveSec=` unless it first clears the list with an empty `OnUnitActiveSec=`.

Audit is `journalctl -u pantry-update`: systemd captures the unit's stdout and stderr plus each run's exit status, so every attempt and its outcome are on the record after the fact with no logging work of our own (Requirement 7.10).

Two consequences that belong in the guide rather than being solved here:

- **Root authority.** A system unit runs as root, which it must in order to drive the Docker daemon. Disclosed in the guide (Requirement 7.12).
- **Unattended deploy.** Enabling the timer means every push to `main` reaches the Pi within an interval with no approval step (Requirement 7.12).

And one that the guide must connect to the existing stdin limitation: recreating the container tears down any `docker attach` session, so the Scan_Listener reads from a stdin nobody is typing into and goes quiet until an operator reattaches (Requirement 7.13). An operator who depends on the barcode listener should leave the timer disabled or accept reattaching after each release.

### 7. Documentation

`deploy/README.md` is the Deployment_Guide, covering: arm64 64-bit Raspberry Pi OS as the supported target; first-time setup (install Docker + Compose plugin, copy `deploy/` to `/opt/pantry`, `cp .env.example .env`, `docker compose up -d`); the update procedure (`docker compose pull && docker compose up -d`); a table of every environment variable with its default and the effect of changing it, noting that `ADDR` and `DB_PATH` are pinned by the compose file and a host port change goes through `HOST_PORT`; scanner attachment plus the stdin limitation; the `pantry-data` volume location (`docker volume inspect pantry-data`) with `docker cp`-based backup and restore steps for `pantry.db`; SHA pinning and rollback via `PANTRY_IMAGE_TAG`; and verification via `curl http://<pi>:8080/health` plus loading `http://<pi>:8080/` from a browser on the same network.

It gains an **Automatic updates (optional)** section for Requirement 7:

- The manual `pull && up -d` procedure is the default; the systemd units are an addition the operator chooses (7.8).
- Install: confirm `command -v docker` is `/usr/bin/docker`, copy both unit files to `/etc/systemd/system/`, `sudo systemctl daemon-reload`, `sudo systemctl enable --now pantry-update.timer`. Disable: `sudo systemctl disable --now pantry-update.timer`. Copying the files without enabling the timer changes nothing (7.7).
- A disclosure block before the enable command, stating two things plainly: the service runs as root because it drives the Docker daemon, and enabling the timer deploys every push to `main` unattended with no approval step (7.12).
- The scan-listener consequence: an update recreates the container and drops any `docker attach` session, so barcode consumption stops until an operator reattaches (7.13). Read together with the stdin limitation section.
- The pinning conflict: if `PANTRY_IMAGE_TAG` names a commit SHA, leave the timer disabled. A SHA tag never moves, so the timer would run forever finding nothing while the operator reasonably believed updates were flowing (7.11). Pinning and auto-updating are opposite intents.
- Interval override: `sudo systemctl edit pantry-update.timer` with a drop-in setting `OnUnitActiveSec=`, not an edit to the installed file. Shorter intervals mean more registry requests and no faster delivery than the release workflow's own runtime (7.6).
- Audit: `journalctl -u pantry-update` for every attempt and outcome, `systemctl list-timers pantry-update.timer` for the next scheduled run (7.10).
- Private-package auth: while `ghcr.io/rhionin/pantry` is public, no credentials are needed. If it is made private, run `docker login ghcr.io` on the Pi with a `read:packages` token; the credentials land in root's Docker config, which is the identity the update service already runs as (7.14).
- Rollback after a bad automatic update: `sudo systemctl disable --now pantry-update.timer`, set `PANTRY_IMAGE_TAG` to the last known-good SHA in `.env`, `docker compose up -d`, confirm `/health`. Disabling the timer is what stops the next attempt from undoing the pin.
- One sentence on a harmless overlap: after an automatic recreate, the operator's next manual `docker compose up -d` may recreate the container one more time; the volume persists either way.

`frontend/README.md` is the Development_Guide, covering the local flow for Requirements 6.9–6.10:

- Two processes, two ports: `npm run dev` in `frontend/` serves the React app with hot module replacement on the Vite port, and `go run ./cmd/server` serves `/api` on the address given by `ADDR`. Browse the Vite port during development.
- The `/api` proxy lives in `frontend/vite.config.ts`; Vite forwards matching requests to the Pantry_Server, which is why the frontend makes same-origin `/api/...` calls in both development and production and needs no environment-dependent base URL.
- Moving between dev-server serving and embedded serving requires no build step and no build flag — it is a matter of which port the browser is pointed at. The Go server always serves whatever is in `internal/webui/assets`.
- Consequently, loading the Go server's address directly during development returns the Placeholder_Assets document, not the live UI. That is expected, and the placeholder names itself as such so the symptom is self-explaining rather than looking like a broken app.

`cmd/server/README.md` gains a short pointer to the Deployment_Guide, so a reader who starts at the server docs finds the deployment path.

### 8. `.gitignore`

```
# Frontend build copied into the Go embed directory
internal/webui/assets/*
!internal/webui/assets/index.html

# Pi deployment secrets/overrides
deploy/.env
```

The glob must be `assets/*`, not `assets/`: git cannot un-ignore a path inside an excluded directory, so ignoring the directory itself would make the `!` negation dead and drop the placeholder from version control.

One accepted wrinkle: a local `docker build` never touches the working tree, but manually copying `frontend/dist/` into the embed directory overwrites the tracked placeholder and shows up as a modified file. `git checkout internal/webui/assets/index.html` restores it. The Dockerfile is the recommended way to produce a production bundle locally, precisely because it leaves the tree clean.

## Data Models

None. The database schema, migrations, and all API payloads are unchanged. The only data-layer change is location: `DB_PATH` moves from a relative `pantry.db` to `/data/pantry.db` inside a Docker volume.

## Error Handling

| Condition | Behavior |
|---|---|
| Non-`GET`/`HEAD` on a web UI path | `405` + `Allow: GET, HEAD`, no body fallback |
| Unregistered `/api/...` path | `404` from `apiMux` (Go's `NotFoundHandler`) |
| Registered `/api` path, wrong method | `405` + `Allow` from `apiMux`, as today |
| Unregistered `/health/...` path | `404` from `apiMux` |
| Asset path stat error or traversal attempt | Falls through to SPA fallback (`200 index.html`) |
| `index.html` unreadable in the injected FS | `500` on fallback paths; cannot occur for the embedded FS, since a missing placeholder is a compile error |
| Frontend build fails in CI or release | Workflow fails; no image pushed; `latest` unchanged |
| Volume not writable by uid 65532 | Server exits at `sql.Open`/migration time; guide documents the named-volume default that avoids it |
| Registry unreachable or errors during `docker compose pull` | The first `ExecStart` exits non-zero, so the unit fails before `up -d` runs; the journal records the failure and the running container is untouched. The timer retries on the next interval |
| Pulled image fails at startup | `restart: unless-stopped` retries into the same failure; the volume and its database are intact. Recovery is `PANTRY_IMAGE_TAG=<last-good-sha>` with `systemctl disable --now pantry-update.timer` |
| `docker` not at the unit's absolute path (not installed, or installed elsewhere) | The service fails on every run with an exec error recorded in the journal; the Pantry_Server keeps running on its current image |
| Private package with no `docker login` on the Pi | `pull` fails unauthorized and the journal records it; `up -d` never runs and the current container keeps running |
| Container recreated while stdin is attached | `docker attach` session ends; Scan_Listener stops consuming barcodes until reattached. No data effect |

## Testing Strategy

**API tests (`internal/server`)** — routing composition, using `handlerTestCase` and `runHandlerTests` per repository convention. Assets come from the tracked placeholder embed, so these tests need no Node.js and no build step. Cases: `GET /` → 200 HTML; `GET /inventory` (client-side route) → 200 same document; `POST /inventory` → 405; `GET /api/nope` → 404; `GET /api` → 404; `DELETE /api/products/1` → 405; `GET /health` → 200 `{"status":"ok"}`; `POST /health` → 405; `GET /health/deep` → 404; plus one existing API route re-asserted to prove precedence is intact.

`httpExchange`'s existing `assertions` field is JSONPath-only, which cannot express "this response is HTML containing X". Two optional fields are added to `httpExchange` — `expectedHeaders map[string]string` and `bodyContains []string` — wired through `buildExpectations`. No new test case type is introduced.

**Unit tests (`internal/webui`)** — everything that needs a *synthetic* asset tree goes here via `NewHandlerFS` over `fstest.MapFS`: byte fidelity across nested trees, `Content-Type` across the extension set, the `index.html` special case, cache headers, method rejection, and traversal-shaped paths. The placeholder embed contains only `index.html`, so injecting a tree is the only way to cover multi-extension asset serving without shipping decoy files inside the production embed directory.

**Property tests** — minimum 100 iterations, tagged `Feature: pi-release-build, Property {n}: {text}`, driven by `pgregory.net/rapid` (already an indirect module dependency) over generated asset trees and request paths.

**Not automated** — workflow triggers, image contents, published tags, compose rendering, unit-file contents, and guide accuracy are one-time verifications, not property tests: they don't vary with input, and most of them test GitHub's, Docker's, or systemd's behavior rather than ours. They are confirmed by the first green CI run, one `docker buildx imagetools inspect`, one `docker compose config`, and a copy-run pass through the guide on the Pi.

The Auto_Update_Units (Requirement 7) fall entirely in that bucket, so **no correctness properties are added for them**. Every criterion is a static unit file, external tool behavior, or guide prose: 7.1–7.3 and 7.6–7.7 are literal keys in two unit files whose contents do not vary with input, 7.4–7.5 and 7.9 are Compose's convergence and Docker's named-volume behavior rather than ours, 7.10 is systemd's journal, and 7.8 plus 7.11–7.14 are documentation content. There is no "for all inputs X" statement to make. Verification is three checks: `systemd-analyze verify` on both unit files, `docker compose config` (a single `pantry` service, no profiles, expected image and volume), and a live pass on the Pi — start the unit twice against an unchanged tag and confirm no recreation, push a commit and confirm the container comes up on the new digest with `/health` responding and the pantry data intact, then read `journalctl -u pantry-update` for both outcomes.

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system-essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Read-only method gate

*For any* request path served by the web UI handler and any HTTP method other than `GET` or `HEAD`, the response status is 405, the `Allow` header lists exactly `GET, HEAD`, and the body is not the `index.html` document.

**Validates: Requirements 3.8**

### Property 2: Embedded assets are served faithfully

*For any* asset tree and *for any* file within it, a `GET` of that file's path returns status 200, a body byte-identical to the file's contents, and a `Content-Type` whose media type matches the file's extension.

**Validates: Requirements 2.4, 3.2, 3.3**

### Property 3: SPA fallback for unmatched paths

*For any* `GET` request path that names no file in the asset tree and does not begin with `/api` or `/health`, the response is status 200 with a body byte-identical to the embedded `index.html` — including the root path `/` and including the case where the asset tree holds only the Placeholder_Assets.

**Validates: Requirements 2.3, 3.1, 3.4**

### Property 4: API paths never fall back to the SPA

*For any* request path beginning with `/api/` that matches no registered API route, the response status is 404 and the body is not the `index.html` document.

**Validates: Requirements 3.5**

### Property 5: Health paths never fall back to the SPA

*For any* request whose path is `/health` or begins with `/health/` and that matches no registered route — for any method — the response status is not 200 and the body is not the `index.html` document.

**Validates: Requirements 3.6**

### Property 6: Mounting the web UI perturbs no registered route

*For any* request matching a route registered on `apiMux`, the response status, headers, and body are identical whether the request is sent to `apiMux` directly or to the composed root handler.

**Validates: Requirements 3.5, 3.6, 3.7**

## Risks and Open Tradeoffs

- **Placeholder overwrite.** Copying a real build into the embed directory dirties a tracked file. Mitigated by building through Docker and documenting the one-command restore.
- **Cross-artifact drift.** Compose defaults duplicate `main.go` defaults. Mitigated by a table-driven check comparing the six documented variables against their `main.go` counterparts.
- **Scanner ergonomics.** `stdin_open`/`tty` do not by themselves deliver host keystrokes to the container. Documented as a limitation rather than solved here; a device-level solution (evdev reader) is out of scope.
- **No image healthcheck.** Distroless has no shell. Accepted; `/health` is verified manually and by any external monitor.
- **Unattended deploy with no human gate.** With the timer enabled, any merge to `main` reaches the Pi within an interval, including a merge that breaks startup. There is no staging environment and no approval step. Mitigated by the timer shipping disabled, by CI gating the release build, and by a documented `PANTRY_IMAGE_TAG` rollback.
- **Root authority.** The update service is a systemd system unit running as root, which is what driving the Docker daemon requires. Its authority is as broad as the daemon's for as long as the timer is enabled. Disclosed in the guide rather than mitigated; the timer being disabled by default is the real control.
- **Recreation versus the scan listener.** The two features actively conflict: an update replaces the container, and an attached scanner session cannot survive that. An operator who relies on the barcode listener should leave the timer disabled, or accept reattaching after each release. Not solvable without the device-level stdin work already out of scope.
