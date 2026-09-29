import { defineConfig, devices } from '@playwright/test'

// Captures screenshots of every screen state for pull request descriptions.
// The API is mocked per test with page.route, so no backend is needed; the
// built SPA is served by `vite preview`. Run with `pnpm screenshots`.
export default defineConfig({
  testDir: './e2e/screenshots',
  outputDir: './test-results/screenshots',
  fullyParallel: true,
  reporter: 'list',
  use: {
    ...devices['Desktop Chrome'],
    baseURL: 'http://127.0.0.1:4173',
    viewport: { width: 1024, height: 640 },
  },
  webServer: {
    command: 'pnpm exec vite preview --host 127.0.0.1 --port 4173 --strictPort',
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: false,
  },
})
