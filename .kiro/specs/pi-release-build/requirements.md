# Requirements Document

## Introduction

Pantry currently runs as two separately started processes during development: a Go API server and a Vite dev server for the React frontend. To run on a Raspberry Pi, the application needs to ship as one self-contained artifact that a Pi can pull and restart without any build tooling installed.

This feature configures the repository so that GitHub Actions validates every change, then on each push to `main` builds a single-architecture `linux/arm64` container image containing a Go binary with the built frontend embedded, and publishes that image to GitHub Container Registry. The repository also gains the Pi-side deployment artifacts (a Compose file, systemd unit files, and operator documentation) needed to run and update the deployment with `docker compose pull && docker compose up -d`. Automatic updates are opt-in: an operator installs and enables the shipped systemd units to run that same pull-and-restart procedure on an interval, and the default deployment applies updates manually.

Scope is limited to build, packaging, static-asset serving, publication, deployment artifacts, and the documentation of the local development flow. No changes to existing API behavior, data model, or frontend features are in scope.

## Glossary

- **Pantry_Server**: The Go HTTP server binary produced from `cmd/server`, serving both the `/api` routes and the embedded frontend.
- **Web_UI_Package**: The Go package (`internal/webui`) that embeds the compiled frontend assets with `//go:embed` and exposes them as an `http.Handler`.
- **SPA_Fallback**: The behavior of returning the embedded `index.html` for a request path that matches no embedded asset file and no server-registered route.
- **Frontend_Build**: The output of `npm run build` in `frontend/`, written to `frontend/dist`.
- **Placeholder_Assets**: Checked-in files inside the Web_UI_Package that satisfy the `//go:embed` directive when no Frontend_Build has been copied in.
- **CI_Workflow**: The GitHub Actions workflow that validates changes on pull requests and pushes.
- **Release_Workflow**: The GitHub Actions workflow that builds and publishes the Container_Image on pushes to `main`.
- **Container_Image**: The `linux/arm64` OCI image containing the Pantry_Server, published to `ghcr.io/rhionin/pantry`.
- **Compose_File**: The Docker Compose file in the repository that a Raspberry Pi operator uses to run the Container_Image.
- **Deployment_Guide**: The repository documentation describing first-time Pi setup and the pull-and-restart update procedure.
- **Scan_Listener**: The existing headless barcode listener in `internal/scanlistener`, which reads barcode lines from the Pantry_Server process's standard input.
- **Data_Volume**: The host directory or named Docker volume mounted into the Container_Image so the SQLite database at `DB_PATH` survives image replacement.
- **Auto_Update_Units**: The systemd oneshot service unit and companion timer unit shipped in the repository, which an operator installs on the Pi to run `docker compose pull` followed by `docker compose up -d` against the Compose_File on a repeating interval.
- **Vite_Dev_Server**: The development server started by `npm run dev` in `frontend/`, which serves the React application with hot module replacement on its own port and proxies `/api` requests to the Pantry_Server.
- **Development_Guide**: The repository documentation in `frontend/README.md` describing the local development flow of the Vite_Dev_Server together with the Pantry_Server.

## Requirements

### Requirement 1

**User Story:** As a contributor, I want GitHub to validate Go and frontend changes on every proposed change, so that broken code is caught before it reaches `main` and gets published to the Pi.

#### Acceptance Criteria

1. WHEN a pull request targeting `main` is opened or updated, THE CI_Workflow SHALL execute the Go test suite by running `./scripts/test-coverage.sh`.
2. WHEN a commit is pushed to `main`, THE CI_Workflow SHALL execute the Go test suite by running `./scripts/test-coverage.sh`.
3. THE CI_Workflow SHALL execute the frontend TypeScript compilation check, the frontend ESLint check, and the frontend Vitest suite in single-run (non-watch) mode.
4. IF any validation step of the CI_Workflow exits with a non-zero status, THEN THE CI_Workflow SHALL report a failed status for the commit.
5. THE CI_Workflow SHALL use the Go toolchain version declared in `go.mod` and the Node.js major version used by the frontend toolchain.

### Requirement 2

**User Story:** As a contributor, I want the Go module to compile and its tests to pass without a frontend build present, so that local development and CI Go jobs do not depend on Node.js.

#### Acceptance Criteria

1. THE Web_UI_Package SHALL include Placeholder_Assets checked into version control that satisfy the `//go:embed` directive.
2. WHEN `go build ./...` runs in a clean checkout with no Frontend_Build present, THE Pantry_Server SHALL compile successfully.
3. WHEN `go test ./...` runs in a clean checkout with no Frontend_Build present, THE Web_UI_Package SHALL serve the Placeholder_Assets rather than failing to initialize.
4. WHERE a Frontend_Build has been copied into the Web_UI_Package embed directory before compilation, THE Web_UI_Package SHALL embed and serve those built assets in place of the Placeholder_Assets.
5. THE repository SHALL exclude copied Frontend_Build output inside the Web_UI_Package embed directory from version control while keeping the Placeholder_Assets tracked.

### Requirement 3

**User Story:** As a Pantry user on my home network, I want to open the Pi's address in a browser and get the full UI, so that I do not need a separate frontend server or port.

#### Acceptance Criteria

1. WHEN a `GET` request for path `/` is received, THE Pantry_Server SHALL respond with status 200 and the embedded `index.html` document.
2. WHEN a `GET` request for an embedded asset path such as `/assets/{hashed_file}` is received, THE Pantry_Server SHALL respond with status 200 and the bytes of that embedded asset.
3. WHEN a `GET` request for an embedded asset is served, THE Pantry_Server SHALL set a `Content-Type` header matching the asset's file extension.
4. WHEN a `GET` request for a client-side route path that matches no embedded asset is received, THE Pantry_Server SHALL apply SPA_Fallback and respond with status 200 and the embedded `index.html` document.
5. IF a request path begins with `/api/` and matches no registered API route, THEN THE Pantry_Server SHALL respond with status 404 without applying SPA_Fallback.
6. IF a request path is `/health` or begins with `/health/` and matches no registered route, THEN THE Pantry_Server SHALL respond with a status other than 200 without applying SPA_Fallback.
7. THE Pantry_Server SHALL serve the embedded frontend and the `/api` routes on the single address given by the `ADDR` environment variable.
8. WHEN a request for a path outside the embedded asset set uses a method other than `GET` or `HEAD`, THE Pantry_Server SHALL respond with status 405 without applying SPA_Fallback.

### Requirement 4

**User Story:** As the repository maintainer, I want each merge to `main` to publish a ready-to-run arm64 image, so that the Pi can be updated without any manual build step.

#### Acceptance Criteria

1. WHEN a commit is pushed to `main` and the CI_Workflow validation succeeds, THE Release_Workflow SHALL build the Container_Image for the `linux/arm64` platform only.
2. THE Release_Workflow SHALL run `npm ci` and `npm run build` in `frontend/` and place the resulting Frontend_Build into the Web_UI_Package embed directory before compiling the Pantry_Server.
3. THE Release_Workflow SHALL compile the Pantry_Server with `CGO_ENABLED=0` and `GOARCH=arm64` and `GOOS=linux`.
4. WHEN the Container_Image build succeeds, THE Release_Workflow SHALL push the image to `ghcr.io/rhionin/pantry` tagged `latest`, tagged `main`, and tagged with the full commit SHA of the pushed commit.
5. THE Release_Workflow SHALL authenticate to GitHub Container Registry using the workflow-scoped `GITHUB_TOKEN` with package write permission.
6. IF the frontend build, the Go compilation, or the image push exits with a non-zero status, THEN THE Release_Workflow SHALL report a failed status and SHALL leave the previously published `latest` tag unchanged.
7. THE Container_Image SHALL run the Pantry_Server as its entrypoint and SHALL contain no build toolchain, Node.js runtime, or source code.
8. THE Container_Image SHALL declare the server listen port and SHALL run the Pantry_Server as a non-root user.

### Requirement 5

**User Story:** As a Raspberry Pi operator, I want a Compose file in the repository that runs the published image correctly, so that setup is a copy-and-run step rather than a configuration exercise.

#### Acceptance Criteria

1. THE Compose_File SHALL reference the `ghcr.io/rhionin/pantry` image and SHALL publish the Pantry_Server port to the Pi host.
2. THE Compose_File SHALL mount a Data_Volume at the directory containing `DB_PATH` so the SQLite database persists across image replacement.
3. THE Compose_File SHALL set `DB_PATH` to a path inside the Data_Volume mount.
4. THE Compose_File SHALL declare a restart policy that restarts the Pantry_Server after a Pi reboot or a process exit.
5. THE Compose_File SHALL enable `stdin_open` and `tty` so the Scan_Listener can read barcode lines from the Pantry_Server process's standard input.
6. THE Compose_File SHALL expose `PRODUCT_CACHE_TTL`, `PRODUCT_MISS_TTL`, `DISABLE_EXTERNAL_PRODUCT_LOOKUP`, `STOCK_IN_CONTROL_BARCODE`, `STOCK_OUT_CONTROL_BARCODE`, `HEADLESS_USER_ID`, `HOST_PORT`, and `PANTRY_IMAGE_TAG` as operator-overridable settings, each with a default value matching the value used in development.
7. THE Compose_File SHALL set `ADDR` and `DB_PATH` to literal pinned values rather than operator-overridable substitutions, so that the Pantry_Server listen port matches the container side of the published port mapping and the SQLite database file resides inside the Data_Volume mount.
8. WHEN an operator sets `HOST_PORT`, THE Compose_File SHALL apply that value to the host side of the published port mapping and SHALL leave the container side of the mapping unchanged.

### Requirement 6

**User Story:** As a Raspberry Pi operator, I want written instructions for installing and updating the deployment, so that I can bring up a new Pi and apply new releases without reading the workflow files.

#### Acceptance Criteria

1. THE Deployment_Guide SHALL state the supported target as 64-bit Raspberry Pi OS on arm64 hardware.
2. THE Deployment_Guide SHALL list the first-time setup steps, including installing Docker and Docker Compose, obtaining the Compose_File, and starting the Pantry_Server.
3. THE Deployment_Guide SHALL document the update procedure as `docker compose pull` followed by `docker compose up -d`.
4. THE Deployment_Guide SHALL document each operator-overridable environment variable exposed by the Compose_File and the effect of changing each one, and SHALL state that `ADDR` and `DB_PATH` are pinned by the Compose_File and that a host port change is made through `HOST_PORT`.
5. THE Deployment_Guide SHALL document how to attach a USB barcode scanner to the Pantry_Server standard input, and SHALL state the limitation that the Scan_Listener consumes barcodes only while the container is run with standard input attached.
6. THE Deployment_Guide SHALL document the Data_Volume location and the steps to back up and restore the SQLite database file.
7. THE Deployment_Guide SHALL document how to pin the deployment to a specific commit-SHA image tag and how to roll back to a previously published tag.
8. THE Deployment_Guide SHALL document verifying a running deployment by requesting `/health` and by loading the UI root path from a browser on the same network.
9. THE Development_Guide SHALL document the local development flow, including starting the Vite_Dev_Server on its own port, starting the Pantry_Server on the address given by `ADDR`, and the `/api` proxy configuration in `frontend/vite.config.ts` that forwards frontend API requests to the Pantry_Server.
10. THE Development_Guide SHALL state that moving between Vite_Dev_Server serving and embedded asset serving requires no build step and no build flag, and SHALL state that requesting the Pantry_Server address directly during local development returns the Placeholder_Assets document rather than the live UI.

### Requirement 7

**User Story:** As a Raspberry Pi operator, I want the Pi to apply new releases on a schedule I opt into, so that a merge to `main` reaches the Pi without me logging in, while the default remains a manual update I control.

#### Acceptance Criteria

1. THE repository SHALL provide the Auto_Update_Units as a systemd oneshot service unit file and a systemd timer unit file under `deploy/systemd/`, installable by an operator on the Pi.
2. THE Compose_File SHALL define the Pantry_Server as its only service and SHALL declare no Compose profile.
3. WHEN the Auto_Update_Units service runs, THE Auto_Update_Units SHALL invoke `docker compose -f <Compose_File> pull` and then `docker compose -f <Compose_File> up -d`, using the Docker Compose CLI that the Deployment_Guide already requires for the manual update procedure.
4. WHEN the `docker compose up -d` step runs and the pulled image digest differs from the digest of the running Pantry_Server container, THE Auto_Update_Units SHALL recreate the Pantry_Server container.
5. WHEN the `docker compose up -d` step runs and the pulled image digest matches the digest of the running Pantry_Server container, THE Auto_Update_Units SHALL leave the running Pantry_Server container in place.
6. THE Auto_Update_Units timer SHALL expose the interval between update attempts as an operator-configurable value with a default of 5 minutes.
7. WHILE the operator has not enabled the Auto_Update_Units timer with `systemctl enable --now`, THE Auto_Update_Units SHALL remain inert and SHALL make no update attempt.
8. THE Deployment_Guide SHALL present the manual pull-and-restart procedure as the default deployment and SHALL present the Auto_Update_Units as an opt-in addition the operator installs and enables with `systemctl enable --now`.
9. WHEN the Auto_Update_Units recreate the Pantry_Server container, THE Data_Volume SHALL be reattached to the new container so the SQLite database file persists.
10. WHEN an update attempt finishes, THE Auto_Update_Units SHALL record the attempt and its outcome in the systemd journal so an operator can audit past attempts after the fact.
11. IF the operator has set `PANTRY_IMAGE_TAG` to a commit-SHA tag, THEN THE Deployment_Guide SHALL direct the operator to leave the Auto_Update_Units timer disabled, because a commit-SHA tag never moves.
12. THE Deployment_Guide SHALL state that the Auto_Update_Units service runs as root on the Pi because the service drives the Docker daemon, and that enabling the timer deploys every push to `main` unattended with no approval step.
13. THE Deployment_Guide SHALL state that recreating the Pantry_Server container drops any attached standard-input session, so the Scan_Listener stops consuming barcodes until an operator reattaches standard input.
14. WHERE the `ghcr.io/rhionin/pantry` package is private, THE Deployment_Guide SHALL document running `docker login ghcr.io` on the Pi with a `read:packages` token as the authentication path, and SHALL state that this step is unnecessary while the package is public.
