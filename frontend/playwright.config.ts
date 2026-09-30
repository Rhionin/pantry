import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { defineConfig } from '@playwright/test'

const apiURL = 'http://127.0.0.1:18080'
const webURL = 'http://127.0.0.1:5173'
const databasePath = join(tmpdir(), `pantry-playwright-${process.pid}.db`)

// Path to the pre-built server binary, relative to this frontend directory.
// CI builds it here (see the "Build server binary" step in .github/workflows/ci.yml)
// so Playwright launches an already-compiled binary instead of cold-compiling
// `go run` under the default webServer timeout. It lives in the repo-root `bin/`
// directory, which is git-ignored. Build locally with:
//   go build -o ../bin/pantry-server ../cmd/server
const serverBinary = join('..', 'bin', 'pantry-server')

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  webServer: [
    {
      command: `DB_PATH=${JSON.stringify(databasePath)} ADDR=127.0.0.1:18080 DISABLE_EXTERNAL_PRODUCT_LOOKUP=true ${JSON.stringify(serverBinary)}`,
      url: `${apiURL}/health`,
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: `VITE_API_PROXY_TARGET=${apiURL} npm run dev -- --host 127.0.0.1`,
      url: webURL,
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
  use: {
    baseURL: webURL,
    locale: 'en-US',
    trace: 'retain-on-failure',
  },
})
