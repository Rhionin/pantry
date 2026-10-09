---
name: verify-pantry
description: >-
  Drive the real Pantry app (Go barcode-inventory server + React/Vite web UI)
  the way a user does and prove a change works: scan barcodes, approve scans,
  check inventory, manage the shopping list, and capture evidence. Reach for
  this when verifying or demonstrating any user-facing Pantry behavior, before
  claiming a fix or feature is done.
---

# Verify Pantry

Pantry is a home barcode-inventory manager. A Go server (`cmd/server`) serves an
HTTP API and, in production, the embedded React build. A user's real surface is
the **web UI** (React + Vite + Mantine, React Router), which talks to the Go API
under `/api`. Products normally come from Open Food Facts; verification disables
that external lookup and seeds products locally instead.

**Primary surface: the web UI.** Three routes cover everyday inventory work:
`/` (Scan Queue), `/inventory` (Inventory), `/shopping` (Shopping List). The
header menu opens `/diagnostics` for page-load timings, manages the Kroger
connection, and shows the version and build id under a Build heading. `Settings` sits
immediately above that Build label and opens `/settings` for the default supply
length and the wipe dialog.
There is also a headless
HID/stdin scanner path and the raw HTTP API; those are secondary and noted in
the feature map, but proofs drive the web UI unless a feature has no UI entry
point.

## Critical environment constraint (read first)

In this sandbox, **background processes and `/tmp` do not survive between separate
shell calls.** Only the repo and `$HOME` (Go build cache, `frontend/node_modules`)
persist. So you cannot "start the server in one command and drive it in the next."
Every verification run must **launch, drive, and tear down inside a single shell
invocation.** `scripts/pantry-verify.sh` does exactly that. If your harness ever
runs on a machine where processes persist, the same script still works; it just
does more than it strictly needs to.

## Prerequisites (one-time, persist in the workspace)

- Go (1.22+) and Node (18+) with npm. Confirm with `doctor` below.
- Frontend deps: `cd frontend && npm install` (already present if
  `frontend/node_modules` exists).
- Playwright's Chromium: `cd frontend && npx playwright install chromium`.
  The drivers use the `@playwright/test` Chromium already vendored in
  `frontend/node_modules`.

## Launch

There is no long-lived server to keep alive here. "Launch" means: build the Go
binary once, then start the API and Vite dev server, drive, and stop them, all in
one `pantry-verify.sh` call.

```bash
# From the repo root. Runs one Playwright driver against a freshly launched app.
bash .agents/skills/verify-pantry/scripts/pantry-verify.sh \
  drive .agents/skills/verify-pantry/scripts/drive-stock-in.mjs stock-in
```

The script:
- builds `cmd/server` to `.agents/skills/verify-pantry/.bin/pantry-server`,
- starts the Go server on `127.0.0.1:18080` with a fresh SQLite DB and
  `DISABLE_EXTERNAL_PRODUCT_LOOKUP=true` (no Open Food Facts calls),
- starts Vite on `127.0.0.1:5173` proxying `/api` to the Go server,
- waits until `GET /health` and the Vite URL answer (that is the readiness
  signal; do not use fixed sleeps),
- runs your driver with `PANTRY_WEB_URL`, `PANTRY_API_URL`, and
  `PANTRY_EVIDENCE_DIR` in its environment,
- tears both servers down on exit.

Ports and the DB path are overridable via `API_PORT`, `WEB_PORT`, `DB_PATH`, but
the defaults are already isolated to a fresh per-run DB.

## Doctor

Read-only preflight that answers "is this checkout worth driving?" It prints tool
versions, builds and boots the Go server, and checks `/health` and
`/api/scanner/config`, then tears down. It writes only into
`evidence/doctor/` and mutates no real state.

```bash
bash .agents/skills/verify-pantry/scripts/pantry-verify.sh doctor
```

A healthy run ends with `PASS: pantry is worth driving` and shows
`health: {"status":"ok"}`. Run this first whenever anything looks off (a driver
hangs, the UI is blank, ports seem taken).

## Drive

Drivers are small Node ES modules under `scripts/` that import `scripts/harness.mjs`
and drive the web UI with Playwright's Chromium. Use **stable handles**, never
coordinates: ARIA roles and accessible names (`getByRole('textbox', { name: 'Barcode
scanner input' })`, `getByRole('button', { name: 'Confirm', exact: true })`),
route links (`getByRole('link', { name: 'Inventory' })`), and visible text.

The key user action is scanning: the barcode field has
`aria-label="Barcode scanner input"`; a driver fills it and presses Enter, which
is exactly what an HID scanner does. `harness.mjs` exposes `scanBarcode`,
`createKnownProduct` (seed a product+barcode override, a legitimate precondition
since external lookup is off), `readInventory` (read-only side-effect check),
`captureProof`, and `assert`.

`scripts/drive-stock-in.mjs` is the worked example: seed a product, scan it in
stock-in mode, approve the pending card, and prove one unit reaches inventory.
Copy its shape for other features. The [feature map](./features/README.md) lists
the recipe for each feature; drive the one you are verifying, not just the
convenient one.

## Evidence

Proof goes to `.agents/skills/verify-pantry/evidence/<feature>/` and survives
cleanup. `captureProof(page, name, sideEffect)` writes three artifacts:

- `<name>.aria.txt` — an ARIA snapshot of the page, showing the app identity and
  the resulting state (e.g. the `Verify Stock-In Milk` / `1 carton` inventory row).
- `<name>.png` — a full-page screenshot.
- `<name>.sideeffect.json` — the structured side effect you verified (e.g. the
  inventory instance count read back from the API).

Proof standards for Pantry:
- Drive the **real user path** (scan + approve), not internal setters or the
  test-only direct API. Seeding a product and reading inventory back are allowed
  as precondition and second-view confirmation, not as the action under test.
- Capture the **action and the resulting state**, not just a final screen: a
  driver asserts the pending card appeared, then that it left the queue, then
  that inventory changed.
- Verify the **side effect**, not only the visible text: confirm the inventory
  row via the API (`readInventory`) as well as on the Inventory route.
- External Open Food Facts lookup is disabled (`DISABLE_EXTERNAL_PRODUCT_LOOKUP=true`),
  which is a real production toggle, so no live third-party calls happen. That is
  the only mocked boundary; everything else is the real server and real SQLite.

## Cleanup

`pantry-verify.sh` tears down on exit via a trap: it kills only the API and Vite
PIDs it started (and Vite's children), never by process name, so it will not
disturb an unrelated server. The fresh per-run SQLite DB lives under the evidence
dir and is discarded next run. **Cleanup never removes the evidence** —
`<name>.aria.txt`, `<name>.png`, and `<name>.sideeffect.json` remain after the
run. `.bin/` (built binary) and `evidence/` are git-ignored (see `.gitignore`);
the committed skill is `SKILL.md`, `features/`, and `scripts/`.

If a driver leaves a stray listener (e.g. it was killed mid-run), re-run any
`pantry-verify.sh` command: it starts on the same fixed ports with `--strictPort`,
so a leftover process surfaces immediately as a bind failure rather than a silent
double-drive.

## Helpers

- `scripts/pantry-verify.sh` — launch/doctor/drive/cleanup wrapper. Invocations
  shown above.
- `scripts/harness.mjs` — Playwright driving helpers (`openBrowser`,
  `createKnownProduct`, `scanBarcode`, `captureProof`, `readInventory`, `assert`).
- `scripts/drive-stock-in.mjs` — worked driver, proven end to end. Template for
  new feature drivers.

## Maintenance

Keep the [feature map](./features/README.md) honest as the UI changes; run
`/maintain-verification-skill` periodically to re-check selectors and coverage.
