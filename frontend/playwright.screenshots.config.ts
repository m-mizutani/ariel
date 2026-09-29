import { defineConfig, devices } from '@playwright/test'

// Captures screenshots of every screen state for pull request descriptions.
// No server runs: e2e/screenshots/states.spec.ts serves the built files in
// dist/ and mocks the API through page.route. Run with `pnpm screenshots`.
export default defineConfig({
  testDir: './e2e/screenshots',
  outputDir: './test-results/screenshots',
  fullyParallel: true,
  reporter: 'list',
  use: {
    ...devices['Desktop Chrome'],
    baseURL: 'http://ariel.test',
    viewport: { width: 1024, height: 640 },
  },
})
