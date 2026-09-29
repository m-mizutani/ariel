import { defineConfig, devices } from '@playwright/test'

// End-to-end tests against the real ariel server. The server runs with
// --no-auth, so a sign-in goes through the real state cookie, callback,
// session, and cookie handling without contacting Slack, and becomes the user
// below. Build the binary first (`task build`), or point ARIEL_BIN at it.
const port = 18081
const baseURL = `http://127.0.0.1:${port}`
const binary = process.env.ARIEL_BIN ?? '../ariel'

export const e2eTeamID = 'T0E2ETEST'
export const e2eUserID = 'U0E2ETEST'
// A Google OAuth client that does not exist. It enables the Google Workspace
// endpoints; the tests stop the browser before it reaches Google.
export const e2eGoogleClientID = 'e2e-client.apps.googleusercontent.com'

export default defineConfig({
  testDir: './e2e/tests',
  outputDir: './test-results/e2e',
  // The server keeps state in memory and the tests share it; run them in one
  // worker so each test's sign-in state is its own browser context only.
  workers: 1,
  reporter: 'list',
  use: {
    ...devices['Desktop Chrome'],
    baseURL,
  },
  webServer: {
    command: [
      binary,
      '--log-format json',
      'serve',
      `--addr 127.0.0.1:${port}`,
      `--base-url ${baseURL}`,
      '--repository-backend memory',
      `--slack-team-id ${e2eTeamID}`,
      `--no-auth ${e2eUserID}`,
      `--google-client-id ${e2eGoogleClientID}`,
      '--google-client-secret e2e-client-secret',
    ].join(' '),
    url: `${baseURL}/login`,
    reuseExistingServer: false,
  },
})
