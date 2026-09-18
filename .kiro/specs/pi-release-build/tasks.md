# Implementation Plan

## Overview

Ship Pantry as one arm64 container image: a Go binary with the compiled React app
embedded, published to `ghcr.io/rhionin/pantry` on every push to `main`, run on a Pi
with `docker compose up -d`.

The work lands in the order that keeps each step independently verifiable with the
tooling already in the repo. `internal/webui` comes first because it is the only piece
with real logic and it is testable in isolation over `fstest.MapFS` — no Node.js, no
Docker, no network. `.gitignore` follows immediately so the tracked placeholder cannot
be lost once a build is copied in. Route composition comes third, once there is a
handler to mount, and is verified through the existing `handlerTestCase` /
`runHandlerTests` framework. Only then do the non-Go artifacts land — Dockerfile, CI,
release, Compose, systemd units, docs — each of which is a one-time verification rather
than something a test asserts.

Verification follows AGENTS.md: API tests in `internal/server/` are the default for the
routing table; unit and `pgregory.net/rapid` property tests live in `internal/webui`
because covering multi-extension asset serving needs a *synthetic* asset tree, which the
production embed directory must not contain. Language: Go (plus YAML, Dockerfile, and
Markdown for the packaging and deployment artifacts).

Note that `pgregory.net/rapid` is currently an **indirect** requirement in `go.mod`. The
first property-test task promotes it to the direct block via `go mod tidy`.

## Tasks

- [x] 1. `internal/webui` — embedded asset serving

  - [x] 1.1 Create the package, the embed directive, and the placeholder asset
    - Create `internal/webui/webui.go` with the package doc comment,
      `//go:embed all:assets` over `var embedded embed.FS`, and both constructors:
      `NewHandler() http.Handler` (calls `fs.Sub(embedded, "assets")`) and
      `NewHandlerFS(assets fs.FS) http.Handler`
    - `all:` is load-bearing: a Vite build emits `.vite/` metadata and can emit
      underscore-prefixed chunk names, which the default `//go:embed` pattern silently
      skips. Comment it so it does not get simplified away
    - Create `internal/webui/assets/index.html` — a minimal tracked document that names
      itself as the placeholder and points at `npm run build`, so a mis-built image is
      obvious in the browser instead of looking like a blank app
    - Read the `index.html` bytes once in the constructor and hold them on the handler
      struct; every fallback response serves that slice
    - `NewHandlerFS` over an FS with no readable `index.html` must not panic — record the
      read error and answer `500` on fallback paths. It cannot happen for the embedded
      tree, where a missing placeholder is a compile error
    - At this point `go build ./...` and `go test ./...` must pass with no frontend build
      present
    - _Requirements: 2.1, 2.2, 2.3_
    - _Properties: 3_

  - [x] 1.2 Implement the method gate and asset serving
    - Method gate runs **first**, before any name resolution or `fs.Stat`: anything other
      than `GET` or `HEAD` gets `405` with `Allow: GET, HEAD` and no body fallback. It
      applies even to paths that do exist as assets — the web UI surface is read-only,
      and gating first is what makes the rule a single sentence to state and to test
    - Name resolution: `name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")`.
      `ServeMux` has already cleaned the path, so traversal-shaped requests arrive
      normalized; anything `fs.Stat` rejects falls through to the fallback rather than
      erroring
    - On a `fs.Stat` hit for a regular file, delegate to `http.FileServerFS(assets)`,
      which supplies `Content-Type` from `mime.TypeByExtension`, range requests, and
      correct `HEAD` handling. Do not hand-roll any of that
    - _Requirements: 3.2, 3.3, 3.8_
    - _Properties: 1, 2_

  - [x] 1.3 Implement SPA fallback
    - Empty name, a directory, a stat error, or a miss → `http.ServeContent(w, r,
      "index.html", time.Time{}, bytes.NewReader(index))`, status 200. `ServeContent`
      derives `text/html; charset=utf-8` from the name and handles `HEAD` correctly
    - Special-case the literal `index.html` path to the fallback branch. Routing it to
      `FileServerFS` instead would trigger that handler's `/index.html` → `./` redirect,
      so the same document would be reachable at two paths with different statuses
    - The zero `time.Time` suppresses `Last-Modified`, which keeps the fallback response
      byte-identical across requests — Property 3 asserts exactly that
    - _Requirements: 2.3, 3.1, 3.4_
    - _Properties: 3_

  - [x] 1.4 Register the missing MIME types and set cache headers
    - A package `init` calls `mime.AddExtensionType` for `.woff2`, `.woff`, and `.ttf`.
      Go's built-in table has none of them and distroless has no `/etc/mime.types` to
      fall back on, so without this fonts ship as `application/octet-stream` — a bug that
      appears only inside the container
    - `Cache-Control: no-cache` on every `index.html` response;
      `Cache-Control: public, max-age=31536000, immutable` for paths under `/assets/`,
      which is Vite's content-hashed output. Without the first header, a browser holding
      a cached `index.html` after an image update requests hashed chunks that no longer
      exist
    - Set the header before delegating to `FileServerFS`/`ServeContent`, since both write
      the status line
    - _Requirements: 3.3_
    - _Properties: 2_

  - [x] 1.5 Property test the read-only method gate
    - **Property 1: Read-only method gate**
    - **Validates: Requirements 3.8**
    - Create `internal/webui/webui_properties_test.go`. Generate an asset tree and a
      request path (existing asset, missing path, `/`, and directory paths all in range)
      plus any method other than `GET`/`HEAD`; assert status 405, `Allow` exactly
      `GET, HEAD`, and a body that is not the `index.html` document
    - Drive it with `NewHandlerFS` over `fstest.MapFS`; minimum 100 iterations; tag
      `Feature: pi-release-build, Property 1: Read-only method gate`
    - Run `go mod tidy` in this task to promote `pgregory.net/rapid` from the indirect to
      the direct require block
    - _Requirements: 3.8_
    - _Properties: 1_

  - [x] 1.6 Property test asset fidelity
    - **Property 2: Embedded assets are served faithfully**
    - **Validates: Requirements 2.4, 3.2, 3.3**
    - In `webui_properties_test.go`: generate a nested tree of files with varied
      extensions (`.js`, `.css`, `.html`, `.svg`, `.png`, `.json`, `.woff2`) and random
      byte contents, then for each file assert `GET` of its path returns 200, a body
      byte-identical to the file contents, and a `Content-Type` whose **media type**
      (parsed with `mime.ParseMediaType`, so a `charset` parameter does not break the
      comparison) matches `mime.TypeByExtension` for that extension
    - Include at least one `.woff2` file in every generated tree — that extension is the
      one the `init` registration exists for, and it is the one that silently regresses
    - Minimum 100 iterations; tagged as above with Property 2
    - _Requirements: 2.4, 3.2, 3.3_
    - _Properties: 2_

  - [x] 1.7 Property test SPA fallback
    - **Property 3: SPA fallback for unmatched paths**
    - **Validates: Requirements 2.3, 3.1, 3.4**
    - In `webui_properties_test.go`: generate an asset tree and a `GET` path that names no
      file in it, and assert status 200 with a body byte-identical to the tree's
      `index.html`. The generator must include `/`, multi-segment client-route shapes
      (`/inventory/123`), and traversal-shaped inputs (`/../etc/passwd`,
      `/assets/../../x`)
    - Run the same property a second time against a tree holding **only**
      `index.html` — that is the Placeholder_Assets case from Requirement 2.3, and it is
      what proves a clean checkout with no frontend build still serves the UI shell
    - Minimum 100 iterations; tagged as above with Property 3
    - _Requirements: 2.3, 3.1, 3.4_
    - _Properties: 3_

  - [x] 1.8 Unit test the cases a property cannot pin down
    - Create `internal/webui/webui_test.go` as a table over `NewHandlerFS`:
      - `/index.html` and `/` return the same status, body, and `Content-Type` (the
        special case from 1.3 — no redirect, no 301)
      - `Cache-Control: no-cache` on `/`, `/index.html`, and a client-route path;
        `public, max-age=31536000, immutable` on `/assets/index-a1b2.js`
      - `HEAD` on an asset and on a fallback path: correct status and headers, empty body
      - `NewHandler()` over the real embedded tree serves the tracked placeholder — the
        one test that asserts the production embed actually resolves
      - an FS whose `index.html` is unreadable answers 500 on a fallback path
    - These are exact single values (a specific header string, a specific status pairing),
      not universally quantified statements, so a table states them more clearly than a
      generator would
    - _Requirements: 2.1, 2.3, 3.1, 3.3_

- [x] 2. Version control hygiene

  - [x] 2.1 Ignore copied build output while keeping the placeholder tracked
    - Append to `.gitignore`:
      `internal/webui/assets/*` then `!internal/webui/assets/index.html`, plus
      `deploy/.env` under its own comment
    - The glob **must** be `assets/*`, not `assets/`: git cannot un-ignore a path inside
      an excluded directory, so ignoring the directory would make the `!` negation dead
      and silently drop the placeholder from version control — which would then break
      `go build` for every fresh clone
    - Verify with `git check-ignore -v` that `internal/webui/assets/index-a1b2.js` is
      ignored and `internal/webui/assets/index.html` is not, and that
      `git status --porcelain` is clean for the placeholder
    - _Requirements: 2.5_

- [x] 3. Route composition in `internal/server`

  - [x] 3.1 Mount the web UI behind a root mux
    - In `internal/server/server.go`, extract today's body into an unexported
      `newAPIMux(catalog, lookupService, refresher, db) (*http.ServeMux, *scan.Queue)`
      that renames the existing local `mux` to `apiMux` and keeps **every** registration
      on it verbatim. `NewHandler` then calls it, builds a `root` mux that `Handle`s
      `/api`, `/api/`, `/health`, and `/health/` back to `apiMux` and `/` to
      `webui.NewHandler()`, and returns `root, scanQueue`
    - The extraction is not cosmetic: Property 6 (task 3.5) has to send the same request
      to the composed handler and to a bare `apiMux`, and `newAPIMux` is the only way an
      in-package test can obtain the latter without duplicating the registration list
    - `NewHandler`'s signature does not change, so `cmd/server/main.go` and the
      `exchanges()` helper need no edits
    - Delegation, not one flat mux, and this is the whole point of the task: Go 1.22+
      precedence would route *registered* paths correctly either way, but `ServeMux`
      synthesizes `405 Method Not Allowed` only when **no** pattern matches. Register `/`
      on the same mux and something always matches, so `DELETE /api/products/1` becomes a
      silent SPA 200. Carving out `/api/` → `NotFoundHandler` fixes the 200 but turns
      that request into a 404, dropping the `405` and `Allow` header today's clients get.
      Nesting preserves both, because `apiMux` still has no catch-all
    - Do **not** wrap the delegation in `http.StripPrefix`. Nested muxes see the
      unmodified path, which is what keeps `PUT /api/products/{id}` and
      `r.PathValue("id")` working
    - _Requirements: 3.5, 3.6, 3.7_
    - _Properties: 4, 5, 6_

  - [x] 3.2 Extend `httpExchange` with header and body-substring expectations
    - Add `expectedHeaders map[string]string` and `bodyContains []string` to
      `httpExchange` in `internal/server/test_runner_test.go` and wire both through
      `buildExpectations` as additional `expect.Assert(...)` calls
    - The existing `assertions` field is JSONPath-only, which cannot express "this
      response is the HTML document containing X" or "this response carries
      `Allow: GET, HEAD`" — the two things every case in 3.3 needs to check
    - Both fields stay optional and zero-valued, so no existing test case changes. Do not
      introduce a second test case type (AGENTS.md)
    - _Requirements: 3.1, 3.4, 3.8_

  - [x] 3.3 API tests for the routing table
    - New `internal/server/handler_webui_test.go` using `handlerTestCase` /
      `runHandlerTests`. Assets come from the tracked placeholder embed, so these tests
      need no Node.js and no build step
    - One case per row of the design's routing table: `GET /` → 200 HTML;
      `GET /inventory` → 200 with a body containing the same placeholder marker (the
      client-side-route case); `POST /inventory` → 405 with `Allow: GET, HEAD`;
      `GET /api/nope` → 404; `GET /api` → 404; `DELETE /api/products/1` → 405;
      `GET /health` → 200 `{"status":"ok"}`; `POST /health` → 405;
      `GET /health/deep` → 404
    - Add one case re-asserting an existing API route through the composed handler
      (`GET /api/products` with a seeded product) to prove precedence survived the
      rewrite
    - Every negative case asserts the body does **not** contain the placeholder marker.
      Status alone is not enough: a 404 body and an SPA body are both plausible-looking
      HTML, and the failure mode this feature introduces is a fallback firing where it
      should not
    - _Requirements: 3.1, 3.2, 3.4, 3.5, 3.6, 3.7, 3.8_
    - _Properties: 1, 4, 5_

  - [x] 3.4 Property test that API and health paths never fall back
    - **Property 4: API paths never fall back to the SPA**
    - **Validates: Requirements 3.5**
    - **Property 5: Health paths never fall back to the SPA**
    - **Validates: Requirements 3.6**
    - New `internal/server/server_properties_test.go` over the composed handler from
      `NewHandler`. Property 4: generate paths under `/api/` that match no registered
      route (random segments, and segments that shadow real prefixes like
      `/api/products/x/y/z`) and assert status 404 with a body that is not the SPA
      document. Property 5: generate `/health` and `/health/...` paths across **all**
      methods and assert the status is never 200 and the body is never the SPA document
    - Both generators skip requests that match a registered route — the properties are
      about *unmatched* paths. For Property 5 that exclusion is load-bearing, not just
      noise reduction: `GET /health` is registered and returns 200, so a generator that
      emits it would fail a correct implementation. Every other method on `/health`, and
      every `/health/...` path, stays in range
    - Minimum 100 iterations each; tagged
      `Feature: pi-release-build, Property 4: ...` / `Property 5: ...`
    - _Requirements: 3.5, 3.6_
    - _Properties: 4, 5_

  - [x] 3.5 Property test that mounting the web UI perturbs no registered route
    - **Property 6: Mounting the web UI perturbs no registered route**
    - **Validates: Requirements 3.5, 3.6, 3.7**
    - In `server_properties_test.go`: build the composed root handler from `NewHandler`
      and a bare `apiMux` from `newAPIMux` (task 3.1), each over its **own** freshly
      migrated and identically seeded database, send each generated request to both, and
      assert status, headers, and body are identical
    - Two databases, not one: a shared database would let a mutating route (`POST
      /api/products`, `POST /api/scans`, the `DELETE`s) observe the first handler's write
      when the second handler runs, so the two responses would legitimately differ and
      the property would fail on a correct implementation
    - Generate over the registered route set (method + path + body, including the
      wildcard routes with generated ids) rather than over free-form strings, so the
      property actually covers the surface it claims to
    - Drop `Date` before comparing headers, and skip `GET /api/events` — it is an SSE
      stream that does not terminate, so a byte comparison would hang rather than fail
    - Normalize server-generated identifiers and timestamps in both bodies before
      comparing (or compare bodies structurally on the fields the route echoes back).
      Routes that mint a UUID or stamp a time return a different byte string on every
      call, which is a property of the handler, not of the routing composition this
      property is about
    - Minimum 100 iterations; tagged as above with Property 6
    - _Requirements: 3.5, 3.6, 3.7_
    - _Properties: 6_

- [~] 4. Checkpoint - the Go side is complete and self-contained
  - `go build ./...` and `go test ./...` pass with no frontend build present, which is
    Requirement 2.2 verified directly
  - Ensure all tests pass, ask the user if questions arise.

- [x] 5. Container packaging

  - [x] 5.1 Write the `Dockerfile`
    - Three stages per the design: `frontend` (`--platform=$BUILDPLATFORM
      node:24-alpine`, `npm ci` from the copied lockfile, then `npm run build`), `build`
      (`--platform=$BUILDPLATFORM golang:1.26-alpine`, `ARG TARGETOS TARGETARCH`,
      `go mod download`, `COPY --from=frontend /src/frontend/dist/ ./internal/webui/assets/`,
      then `CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath
      -ldflags="-s -w"`), and `gcr.io/distroless/static-debian12:nonroot` as runtime
    - `--platform=$BUILDPLATFORM` on **both** build stages is what keeps the release
      fast: the Go toolchain cross-compiles natively, so the runner's amd64 CPU does the
      work and `TARGETARCH=arm64` selects the output. Omit it and `npm ci` plus a Vite
      build run under QEMU, turning seconds into minutes. The runtime stage deliberately
      has no `--platform`, so the produced manifest is arm64
    - Copy the `package.json`/`package-lock.json` pair before the rest of `frontend/` so
      the `npm ci` layer caches independently of source edits
    - `RUN mkdir -p /data` in the build stage, then
      `COPY --from=build --chown=nonroot:nonroot /data /data` — distroless has no shell,
      so the directory cannot be created in the runtime stage. A Docker *named* volume
      inherits ownership from the image path, which is how uid 65532 can write the
      database with no operator action
    - `ENV ADDR=":8080" DB_PATH="/data/pantry.db"`, `EXPOSE 8080`, `VOLUME ["/data"]`,
      `USER nonroot:nonroot`, `ENTRYPOINT ["/usr/local/bin/pantry-server"]`
    - No `HEALTHCHECK`: distroless ships no shell, `curl`, or `wget`, so it cannot be
      expressed. `/health` verification lives in the deployment guide instead
    - Add `# syntax=docker/dockerfile:1` as the first line
    - _Requirements: 4.2, 4.3, 4.7, 4.8_

- [x] 6. Validation workflow

  - [x] 6.1 Pin the Node major version
    - Create `frontend/.nvmrc` containing `24`, matching `@types/node ^24` and Vite 8
    - This is the file `actions/setup-node` reads via `node-version-file`, so it is what
      satisfies "the Node.js major version used by the frontend toolchain" without
      hardcoding a version in the workflow
    - _Requirements: 1.5_

  - [x] 6.2 Write `.github/workflows/ci.yml`
    - Triggers: `pull_request` targeting `main`, `push` to `main`, and `workflow_call` so
      the release workflow can gate on it. `permissions: contents: read`
    - Job `go`: `actions/checkout@v4`, `actions/setup-go@v5` with
      `go-version-file: go.mod` (this is how Requirement 1.5's Go half is satisfied —
      never a literal version), a one-line
      `sudo apt-get update && sudo apt-get install -y bc` guard, then
      `./scripts/test-coverage.sh`
    - The `bc` guard is not redundant paranoia: the script's threshold comparison is
      `echo "..." | bc -l`, and without `bc` that subshell fails in a way the script does
      not surface as a coverage failure. `bc` is present on today's `ubuntu-latest`, and
      the guard is what keeps the job correct when that changes
    - Job `frontend` (parallel): `actions/setup-node@v4` with
      `node-version-file: frontend/.nvmrc` and `cache: npm`, `npm ci`, `npx tsc -b`,
      `npm run lint`, `npx vitest --run`. Use `--run` explicitly — `npm test` is bare
      `vitest`, which watches and would hang the job
    - No step may use `|| true`, `continue-on-error`, or a pipe that masks an exit
      status. That absence is the entire mechanism behind Requirement 1.4
    - The coverage script rewrites its own `COVERAGE_THRESHOLD` when coverage rises; in
      CI that dirties a file nobody commits, which is harmless and expected
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5_

- [x] 7. Release workflow

  - [x] 7.1 Write `.github/workflows/release.yml`
    - Trigger: `push` to `main`. `permissions: contents: read, packages: write` — the
      `packages: write` scope is what lets the workflow-scoped `GITHUB_TOKEN`
      authenticate to GHCR, with no PAT anywhere
    - Job `validate`: `uses: ./.github/workflows/ci.yml`. Job `publish`:
      `needs: validate`, so a red test suite blocks the push. A `workflow_run` trigger
      would fire asynchronously and could publish an unvalidated commit
    - `publish` steps: `actions/checkout@v4`, `docker/setup-qemu-action@v3` (optional —
      the runtime stage only `COPY`s and executes no arm64 instruction; keep it as cheap
      insurance for a future runtime `RUN`), `docker/setup-buildx-action@v3`,
      `docker/login-action@v3` against `ghcr.io` with `${{ github.actor }}` and
      `${{ secrets.GITHUB_TOKEN }}`, then `docker/build-push-action@v6` with
      `context: .`, `platforms: linux/arm64`, `push: true`, `provenance: false`,
      `cache-from: type=gha`, `cache-to: type=gha,mode=max`
    - Tags as a literal three-line list: `:latest`, `:main`, and
      `:${{ github.sha }}`. Not `docker/metadata-action` — its `type=sha` emits a
      `sha-`-prefixed and by default *short* tag, and Requirement 4.4 asks for the full
      SHA
    - One build-push step, so a failure in the frontend build, the Go compile, or the push
      leaves `latest` on the previous digest. Nothing pushes incrementally
    - `provenance: false` keeps the published reference a plain single-platform image
      rather than an attestation index, which older Docker versions on a Pi handle less
      predictably
    - _Requirements: 4.1, 4.4, 4.5, 4.6_

- [x] 8. Pi deployment artifacts

  - [x] 8.1 Write `deploy/docker-compose.yml`
    - No top-level `version:` key. It is obsolete under Compose v2 and modern Docker
      warns about it on every command — noise on exactly the box where the operator is
      least able to tell noise from an error
    - One `pantry` service on `ghcr.io/rhionin/pantry:${PANTRY_IMAGE_TAG:-latest}`, with
      `restart: unless-stopped`, `stdin_open: true`, `tty: true`,
      `ports: ["${HOST_PORT:-8080}:8080"]`, and a named volume `pantry-data` mounted at
      `/data`. `pantry` is the file's only service and no `profiles:` key appears anywhere
    - Environment: `ADDR: ":8080"` and `DB_PATH: /data/pantry.db` **pinned literally**,
      then `PRODUCT_CACHE_TTL`, `PRODUCT_MISS_TTL`,
      `DISABLE_EXTERNAL_PRODUCT_LOOKUP`, `STOCK_IN_CONTROL_BARCODE`,
      `STOCK_OUT_CONTROL_BARCODE`, and `HEADLESS_USER_ID` as `${VAR:-default}`
      substitutions with the development defaults (`720h`, `168h`, `false`, `STOCK_IN`,
      `STOCK_OUT`, `user-1`)
    - `ADDR` and `DB_PATH` are pinned on purpose: `ADDR`'s port has to agree with the
      container side of the port mapping, and `DB_PATH` has to sit inside the volume
      mount. Making either operator-overridable is a silent-data-loss footgun. Host-side
      port changes go through `HOST_PORT`
    - A **named** volume, not a bind mount: a named volume inherits `/data`'s ownership
      from the image so uid 65532 can write, while a bind mount does not
    - Declaring `pantry-data` under `volumes:` and referencing it by name is what makes any
      container replacement safe — manual or timer-driven. Every recreation reattaches the
      same volume and `pantry.db` is untouched. The database must never land in the
      container's own writable layer
    - `HOST_PORT` substitutes only the left half of `"${HOST_PORT:-8080}:8080"`; the
      container side stays `8080` and therefore stays in agreement with the pinned `ADDR`
    - `PANTRY_IMAGE_TAG` is the mechanism behind SHA pinning and rollback in 9.1
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8, 7.2, 7.9_

  - [x] 8.2 Write `deploy/.env.example`
    - Every variable the Compose file substitutes — `PANTRY_IMAGE_TAG`, `HOST_PORT`, and
      the six application settings, eight names in total — each with its default value and
      a one-line comment on the effect of changing it
    - Compose auto-loads `.env` from the compose file's directory, so the operator's step
      is `cp .env.example .env`. `deploy/.env` is git-ignored by task 2.1
    - Do not list `ADDR` or `DB_PATH` here; they are pinned in the Compose file and
      documenting them as knobs would invite exactly the misconfiguration 8.1 avoids
    - _Requirements: 5.6, 6.4_

  - [x] 8.3 Test the Compose defaults against `main.go`
    - Table-driven test in `cmd/server/main_test.go` asserting each of the six
      documented Compose defaults equals its `main.go` counterpart:
      `PRODUCT_CACHE_TTL`=`defaultProductCacheTTL`, `PRODUCT_MISS_TTL`=`defaultMissTTL`,
      and the `loadScanListenerConfig` defaults for the control barcodes and
      `HEADLESS_USER_ID`, plus `DISABLE_EXTERNAL_PRODUCT_LOOKUP` defaulting to enabled
    - This closes the cross-artifact drift risk the design names: the Compose file
      duplicates values that live in Go, and nothing else in the build would notice when
      one side moves. Read the expected values from the Go constants and the actual ones
      from `deploy/docker-compose.yml`, so the test fails on drift in either direction
    - _Requirements: 5.6_

  - [x] 8.4 Write the Auto_Update_Units in `deploy/systemd/`
    - Two new files per design §6: `deploy/systemd/pantry-update.service` and
      `deploy/systemd/pantry-update.timer`. Neither is referenced by the Compose file and
      neither has any effect until an operator installs and enables it (9.1 covers that)
    - Service: `Type=oneshot` with **two** `ExecStart=` lines —
      `docker compose -f /opt/pantry/docker-compose.yml pull` then the same with `up -d`.
      systemd runs them in order and a non-zero exit from the pull aborts the unit before
      `up -d`, which is what keeps a failed pull from triggering a restart
    - `TimeoutStartSec=15min`. The directive covers the whole `ExecStart` sequence of a
      oneshot unit and defaults to **90 seconds** — short enough that a first image pull
      over a slow home connection gets SIGTERMed mid-transfer and the unit fails on every
      attempt. Finite rather than `infinity`, because a pull that never returns would keep
      the unit active forever and the timer will not start a second instance while the
      first is still running, so every later update would be blocked
    - Absolute `/usr/bin/docker` in both lines. `ExecStart` is not run through a shell and
      systemd does not resolve `PATH` the way a login shell does, so a bare `docker` fails
      with a 203/EXEC error on every run
    - `WorkingDirectory=/opt/pantry`. A systemd-run unit has no shell cwd — it starts in
      `/` — and Compose resolves `.env` relative to the compose file's directory. Setting
      both to the same directory is what makes the operator's `.env` overrides apply to the
      automatic path; without it the deployment silently falls back to every compose default
    - `Wants=` and `After=` both listing `network-online.target docker.service`
    - Timer: `OnBootSec=2min`, `OnUnitActiveSec=5min`, `Unit=pantry-update.service`, and
      `[Install] WantedBy=timers.target` — the `[Install]` section is what makes
      `systemctl enable` meaningful. `OnBootSec` is what gets a Pi that has been off for a
      week to check shortly after boot instead of waiting out a full interval
    - Do **not** set `Persistent=true`. It only replays missed *calendar* triggers
      (`OnCalendar=`) and does nothing for a monotonic `OnUnitActiveSec=`; `OnBootSec=`
      already covers the reboot case
    - `up -d` is the idempotence mechanism: Compose recreates only when the pulled digest
      differs, so an unchanged tag is a no-op. That is what makes a 5-minute interval safe
    - Verify both files with `systemd-analyze verify` before moving on
    - _Requirements: 7.1, 7.3, 7.4, 7.5, 7.6, 7.7, 7.9, 7.10_

- [x] 9. Deployment documentation

  - [x] 9.1 Write `deploy/README.md` as the Deployment_Guide
    - Supported target stated up front: 64-bit Raspberry Pi OS on arm64 hardware
    - First-time setup: install Docker and the Compose plugin, copy the `deploy/`
      directory to `/opt/pantry` on the Pi, `cp .env.example .env`, `docker compose up -d`.
      Name `/opt/pantry` concretely — it is the path the Auto_Update_Units hardcode, so an
      operator who deploys elsewhere has to edit both `ExecStart` lines and
      `WorkingDirectory` to match
    - Update procedure: `docker compose pull` followed by `docker compose up -d`
    - A table of every environment variable with its default and the effect of changing
      it, covering the eight names in `.env.example`
    - Scanner attachment via `docker attach pantry`, and the limitation stated plainly:
      `stdin_open` and `tty` keep the container's stdin open, but a USB HID scanner types
      into the *Pi's* console, not into the container, so barcodes are consumed only while
      stdin is actually attached
    - Data volume: locate it with `docker volume inspect pantry-data`; back up and restore
      `pantry.db` with `docker cp`. Also document `chown 65532:65532` for an operator who
      switches to a bind mount, since a bind mount does not inherit the image's ownership
      and the server would exit at migration time
    - Pinning and rollback: set `PANTRY_IMAGE_TAG` to a full commit SHA in `.env`, then
      `docker compose up -d`; rolling back is the same step with an earlier SHA
    - Verification: `curl http://<pi>:8080/health` and loading `http://<pi>:8080/` in a
      browser on the same network. This is where `/health` checking lives, since the
      distroless image cannot carry a `HEALTHCHECK`
    - An **"Automatic updates (optional)"** section per design §7:
      - State that the manual `pull && up -d` procedure is the default and the
        Auto_Update_Units are an addition the operator chooses
      - Install: confirm `command -v docker` reports `/usr/bin/docker`, copy both unit
        files to `/etc/systemd/system/`, `sudo systemctl daemon-reload`,
        `sudo systemctl enable --now pantry-update.timer`. Disable:
        `sudo systemctl disable --now pantry-update.timer`. Say plainly that copying the
        files without enabling the timer changes nothing
      - A disclosure block placed **before** the enable command, stating two things
        plainly: the service runs as root because it drives the Docker daemon, and enabling
        the timer deploys every push to `main` unattended with no approval step
      - The scan-listener consequence: an update recreates the container and drops any
        `docker attach` session, so barcode consumption stops until an operator
        reattaches. Cross-reference the existing stdin limitation section
      - The SHA-pin conflict: if `PANTRY_IMAGE_TAG` names a commit SHA, leave the timer
        disabled. A SHA tag never moves, so the timer runs forever finding nothing while
        the operator reasonably believes updates are flowing — pinning and auto-updating
        are opposite intents
      - Interval override through a drop-in: `sudo systemctl edit pantry-update.timer`,
        not an edit to the installed unit file. Include the gotcha that a drop-in *adds*
        to `OnUnitActiveSec=` unless it first clears the list with an empty
        `OnUnitActiveSec=`. Shorter intervals buy more registry requests, not faster
        delivery than the release workflow's own runtime
      - Audit: `journalctl -u pantry-update` for every attempt and its outcome,
        `systemctl list-timers pantry-update.timer` for the next scheduled run
      - Private-package auth as a conditional: no credentials are needed while
        `ghcr.io/rhionin/pantry` is public; if it is made private, run
        `docker login ghcr.io` on the Pi with a `read:packages` token. Note that the
        credentials land in root's Docker config, which is the identity the update service
        already runs as. State the condition, so a reader on a public package does not
        configure a token they do not need
      - Rollback after a bad automatic update, in this order:
        `sudo systemctl disable --now pantry-update.timer`, set `PANTRY_IMAGE_TAG` to the
        last known-good SHA in `.env`, `docker compose up -d`, confirm `/health`. Disabling
        the timer first is what stops the next attempt from undoing the pin
      - One sentence on a harmless overlap: after an automatic recreate, the operator's
        next manual `docker compose up -d` may recreate the container one more time; the
        volume persists either way
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5, 6.6, 6.7, 6.8, 7.8, 7.11, 7.12, 7.13, 7.14_

  - [x] 9.2 Point `cmd/server/README.md` at the deployment guide
    - Add a short "Deployment" section linking to `deploy/README.md`, so a reader who
      starts at the server docs finds the Pi path instead of inferring it from the
      workflow files
    - _Requirements: 6.2_

  - [x] 9.3 Write the Development_Guide in `frontend/README.md`
    - `frontend/README.md` is currently the stock Vite template boilerplate (plugin list,
      React Compiler note, ESLint expansion snippets). Replace it, or prepend a
      "Local development" section ahead of it, so the first thing a contributor reads is
      how to run the app rather than how to configure type-aware lint rules
    - Two processes, two ports: `npm run dev` in `frontend/` serves the React app with hot
      module replacement on the Vite port, and `go run ./cmd/server` serves `/api` on the
      address given by `ADDR`. Browse the **Vite** port during development
    - The `/api` proxy lives in `frontend/vite.config.ts` and forwards matching requests to
      the Go server. State why it matters: the frontend makes same-origin `/api/...` calls
      in both development and production, so it needs no environment-dependent base URL
    - Switching between dev-server serving and embedded serving needs no build step and no
      build flag — it is only a question of which port the browser hits. The Go server
      always serves whatever is in `internal/webui/assets`
    - Consequently, hitting the Go port directly during development returns the
      placeholder document, not the live UI. Say that this is expected and that the
      placeholder names itself, so the symptom explains itself instead of reading as a
      broken app
    - Note that the Playwright setup already runs both processes and needs no change:
      `frontend/playwright.config.ts` boots `go run ../cmd/server` on `127.0.0.1:18080`
      with a temp-file `DB_PATH` and the Vite dev server on `127.0.0.1:5173`, pointing the
      proxy at the API through `VITE_API_PROXY_TARGET`. A contributor who reads the
      two-process instructions should not conclude the e2e suite needs manual setup
    - _Requirements: 6.9, 6.10_

- [ ] 10. Final verification

  - [~] 10.1 Run the Go suite and the coverage gate
    - `go test ./...`, then `go test -race ./internal/webui/... ./internal/server/...`,
      then `./scripts/test-coverage.sh`
    - Commit the updated script if the threshold ratcheted up, per AGENTS.md
    - _Requirements: 1.1, 2.2, 2.3_

  - [~] 10.2 Run the frontend checks the CI job runs
    - In `frontend/`: `npm ci`, `npx tsc -b`, `npm run lint`, `npx vitest --run`
    - Running them locally with the same flags is what confirms the `ci.yml` `frontend`
      job will pass rather than discovering it on the first push
    - _Requirements: 1.3_

  - [~] 10.3 Validate the Compose file renders
    - `docker compose -f deploy/docker-compose.yml config` with `deploy/.env` absent, and
      again with a copy of `.env.example` in place; confirm the resolved output pins
      `ADDR` and `DB_PATH`, maps the host port, and carries all six application settings
      with their expected defaults
    - Confirm the command emits no obsolete-`version` warning, which is the observable
      check on 8.1's omission of the key
    - Confirm the render contains a **single** `pantry` service with no `profiles:` key on
      it — that is Requirement 7.2, and it is the one thing a YAML review can miss by eye
    - Compose rendering is a one-time verification of Docker's substitution behavior, not
      something a property test should own
    - _Requirements: 5.1, 5.2, 5.3, 5.6, 7.2_

  - [~] 10.4 Sanity-check the image build locally
    - `docker buildx build --platform linux/arm64 -t pantry:local .`, then
      `docker buildx imagetools inspect` (or `docker image inspect`) to confirm the
      manifest is arm64 and that `ENTRYPOINT`, `EXPOSE`, `USER`, and `VOLUME` are set
    - Optional because it needs a working buildx with a registry or local exporter, which
      not every environment has. The first green run of `release.yml` verifies the same
      thing; this task only shortens the feedback loop
    - _Requirements: 4.1, 4.2, 4.3, 4.7, 4.8_

  - [~] 10.5 Validate the Auto_Update_Units
    - `systemd-analyze verify deploy/systemd/pantry-update.service deploy/systemd/pantry-update.timer`
      — catches a misspelled directive, a key in the wrong section, and a malformed time
      span, none of which a read-through reliably catches. It does **not** check for a
      missing `[Install]` section, and it should not: the service unit deliberately has
      none, since the timer is what activates it
    - Two warnings are expected rather than failures when the check runs on a box without
      Docker installed: `/usr/bin/docker` not being executable, and `docker.service` not
      resolving as a dependency. Both are properties of the checking host, not of the units
    - Then, on a Pi or any Linux box with Docker: `systemctl start pantry-update.service`
      twice against an unchanged tag and confirm the second run recreates nothing, and that
      `journalctl -u pantry-update` records both attempts and their exit status
    - Optional because the whole task needs a Linux host with systemd. The development
      environment here is macOS, where `systemd-analyze` does not exist, so neither half
      runs locally. It moves to the Pi alongside the manual acceptance step in the notes
    - _Requirements: 7.1, 7.3, 7.5, 7.6, 7.7, 7.10_

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation

**Ordering.** Waves follow file dependencies, not task numbers. Tasks 1.1–1.4 all edit
`internal/webui/webui.go` and 1.5–1.7 all edit `internal/webui/webui_properties_test.go`,
so each is its own wave. Same for 3.4 and 3.5 in
`internal/server/server_properties_test.go`. Task 3.2 (the widened `httpExchange`) must
land before 3.3, 3.4, and 3.5, since none of their assertions are expressible without
`expectedHeaders` and `bodyContains`. The infra artifacts (5.1, 6.x, 7.1, 8.x, 9.x) touch
no Go file and depend on nothing in section 1 or 3, so they fill out the early waves in
parallel. Task 8.3 is the exception in that group: it is a Go test in
`cmd/server/main_test.go` and needs 8.1 and 8.2 in place first. Task 8.4 writes new files
under `deploy/systemd/` and so conflicts with nothing, but it stays one wave after 8.1
because its `ExecStart` paths have to name the compose file that 8.1 creates. 9.3 is the
only task touching `frontend/README.md` and depends on nothing, so it sits in an early wave.

**Why the property tests split across two packages.** Properties 1–3 need a *synthetic*
asset tree — multiple extensions, nested directories, generated bytes — and the production
embed directory must contain only the placeholder, so they live in `internal/webui` over
`NewHandlerFS` and `fstest.MapFS`. Properties 4–6 are statements about route composition
and only exist once `apiMux` and the web UI are mounted together, so they live in
`internal/server` over the real `NewHandler`. Neither package's tests need Node.js or a
frontend build.

**Verifying task per property.**
1 → 1.5, 3.3; 2 → 1.6; 3 → 1.7; 4 → 3.4, 3.3; 5 → 3.4, 3.3; 6 → 3.5.

**Deliberately not automated.** Workflow triggers, published tag names, image contents,
unit-file contents, and guide accuracy do not vary with input and mostly test GitHub's,
Docker's, or systemd's behavior rather than ours. They are confirmed by the first green CI
run, tasks 10.3 through 10.5, and a copy-run pass through `deploy/README.md` on the Pi.

**Requirement 7 has no correctness properties.** The Auto_Update_Units are two static
systemd unit files plus guide prose — there is no input to quantify over, and the behavior
under test belongs to systemd, Docker, and Compose rather than to Pantry. `up -d`'s
recreate-only-on-digest-change and the named volume's reattachment are Docker's guarantees,
the journal is systemd's, and the rest is documentation content. What is checkable
statically is checked by `systemd-analyze verify` and the single-service `docker compose
config` render in tasks 10.5 and 10.3.

**One manual acceptance step on the Pi.** Enabling the timer with
`systemctl enable --now pantry-update.timer`, pushing a commit to `main`, and then
confirming that the `pantry` container is recreated on the new digest, that `/health`
responds afterward, that the existing inventory data survived the recreation, and that
`journalctl -u pantry-update` shows both a no-op run and an applied run. Nothing short of a
live registry, a live Docker daemon, a real image push, and a running systemd exercises that
path, so it stays an operator check rather than a task a coding agent can close.

**The one accepted wrinkle.** Manually copying `frontend/dist/` into
`internal/webui/assets/` overwrites the tracked placeholder and shows up as a modified
file; `git checkout internal/webui/assets/index.html` restores it. Building through the
Dockerfile is the recommended local path precisely because it never touches the working
tree.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "2.1", "6.1"] },
    { "id": 1, "tasks": ["1.2", "5.1", "9.3"] },
    { "id": 2, "tasks": ["1.3", "6.2", "8.1"] },
    { "id": 3, "tasks": ["1.4", "7.1", "8.2", "8.4"] },
    { "id": 4, "tasks": ["1.5", "3.1", "9.1"] },
    { "id": 5, "tasks": ["1.6", "3.2", "9.2"] },
    { "id": 6, "tasks": ["1.7", "3.3", "8.3"] },
    { "id": 7, "tasks": ["1.8", "3.4"] },
    { "id": 8, "tasks": ["3.5"] },
    { "id": 9, "tasks": ["4"] },
    { "id": 10, "tasks": ["10.1", "10.2", "10.3", "10.4", "10.5"] }
  ]
}
```
