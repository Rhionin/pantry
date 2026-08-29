import { defineConfig } from '@playwright/test'

// E2E tests (task 14.x) live in e2e/ and drive the app through a dev server.
export default defineConfig({
  testDir: './e2e',
  webServer: {
    command: 'npm run dev',
    url: 'http://localhost:5173',
    reuseExistingServer: true,
  },
  use: {
    baseURL: 'http://localhost:5173',
  },
})
