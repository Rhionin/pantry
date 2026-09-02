import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { defineConfig } from '@playwright/test'

const apiURL = 'http://127.0.0.1:18080'
const webURL = 'http://127.0.0.1:5173'
const databasePath = join(tmpdir(), `pantry-playwright-${process.pid}.db`)

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  webServer: [
    {
      command: `DB_PATH=${JSON.stringify(databasePath)} ADDR=127.0.0.1:18080 DISABLE_EXTERNAL_PRODUCT_LOOKUP=true go run ../cmd/server`,
      url: `${apiURL}/health`,
      reuseExistingServer: false,
    },
    {
      command: `VITE_API_PROXY_TARGET=${apiURL} npm run dev -- --host 127.0.0.1`,
      url: webURL,
      reuseExistingServer: false,
    },
  ],
  use: {
    baseURL: webURL,
    locale: 'en-US',
    trace: 'retain-on-failure',
  },
})
