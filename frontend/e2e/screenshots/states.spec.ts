import { readFile } from 'node:fs/promises'
import { extname, join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

// One test per screen state listed in the spec. Each test mocks the API the
// state depends on, waits for the state to be visible, and saves
// screenshots/<name>.png.

const distDir = join(process.cwd(), 'dist')
const contentTypes: Record<string, string> = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
}

// Serve the built SPA the way the Go server does: a file when it exists,
// index.html otherwise. Tests register their API mocks afterwards, and
// Playwright tries the most recently registered route first.
test.beforeEach(async ({ page }) => {
  await page.route('http://ariel.test/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path.startsWith('/api/')) {
      await route.fulfill({ status: 404, contentType: 'application/json', body: '{"error":"not_found"}' })
      return
    }
    const file = path === '/' ? 'index.html' : path.slice(1)
    try {
      const body = await readFile(join(distDir, file))
      await route.fulfill({ status: 200, contentType: contentTypes[extname(file)] ?? 'application/octet-stream', body })
    } catch {
      const body = await readFile(join(distDir, 'index.html'))
      await route.fulfill({ status: 200, contentType: 'text/html', body })
    }
  })
})

// Resolved against the working directory, which is frontend/ for `pnpm screenshots`.
const shot = (name: string) => ({ path: `screenshots/${name}.png`, fullPage: true })

const me = (connected: boolean) =>
  JSON.stringify({ team_id: 'T0123ABCD', user_id: 'U0123ABCD', name: 'Alice Example', slack_connected: connected })

// Some states are captured while a request is still pending, so those route
// handlers never answer. Drop them after each test; otherwise closing the
// page waits for them forever.
test.afterEach(async ({ page }) => {
  await page.unrouteAll({ behavior: 'ignoreErrors' })
})

async function mockMe(page: Page, status: number, body: string) {
  await page.route('**/api/auth/me', (route) =>
    route.fulfill({ status, contentType: 'application/json', body }),
  )
}

async function signedOut(page: Page) {
  await mockMe(page, 401, '{"error":"unauthenticated"}')
}

const googleNotConnected = { available: true, connected: false, email: '' }
const googleConnected = { available: true, connected: true, email: 'alice@example.com' }

async function mockGoogle(page: Page, status: number, body: object) {
  await page.route('**/api/integrations/google-workspace', (route) =>
    route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) }),
  )
}

// The settings page of a user linked to Slack, with the given Google Workspace
// status.
async function settingsWithGoogle(page: Page, google: object) {
  await mockMe(page, 200, me(true))
  await mockGoogle(page, 200, google)
}

test('login: initial', async ({ page }) => {
  await signedOut(page)
  await page.goto('/login')
  await expect(page.getByRole('button', { name: 'Sign in with Slack' })).toBeVisible()
  await page.screenshot(shot('login-initial'))
})

test('login: redirecting to Slack', async ({ page }) => {
  await signedOut(page)
  // Answer the navigation to /api/auth/login with 204: the browser keeps the
  // current document, so the page stays in its redirecting state. A request
  // left pending instead keeps the navigation open and blocks the test.
  await page.route('**/api/auth/login', (route) => route.fulfill({ status: 204 }))
  await page.goto('/login')
  // The click starts a navigation that never completes; do not wait for it.
  await page.getByRole('button', { name: 'Sign in with Slack' }).click({ noWaitAfter: true })
  await expect(page.getByRole('button', { name: 'Redirecting to Slack…' })).toBeDisabled()
  await page.screenshot(shot('login-redirecting'))
})

test('login: cancelled at Slack', async ({ page }) => {
  await signedOut(page)
  await page.goto('/login?error=access_denied')
  await expect(page.getByRole('alert')).toContainText('Sign-in was cancelled.')
  await page.screenshot(shot('login-cancelled'))
})

test('login: failed', async ({ page }) => {
  await signedOut(page)
  await page.goto('/login?error=login_failed')
  await expect(page.getByRole('alert')).toContainText('Sign-in failed.')
  await page.screenshot(shot('login-failed'))
})

test('sign-in check: loading', async ({ page }) => {
  await page.route('**/api/auth/me', () => new Promise(() => {}))
  await page.goto('/')
  await expect(page.getByText('Loading…')).toBeVisible()
  await page.screenshot(shot('check-loading'))
})

test('sign-in check: failed', async ({ page }) => {
  await mockMe(page, 500, '{"error":"internal_error"}')
  await page.goto('/')
  await expect(page.getByRole('alert')).toContainText('Could not check your sign-in status.')
  await page.screenshot(shot('check-failed'))
})

test('settings: Slack connected', async ({ page }) => {
  await mockMe(page, 200, me(true))
  await mockGoogle(page, 200, googleNotConnected)
  await page.goto('/settings')
  await expect(page.getByRole('listitem', { name: 'Slack' }).getByText('Connected')).toBeVisible()
  await page.screenshot(shot('settings-slack-connected'))
})

test('settings: Slack not connected', async ({ page }) => {
  await mockMe(page, 200, me(false))
  await mockGoogle(page, 200, googleNotConnected)
  await page.goto('/settings')
  await expect(page.getByRole('button', { name: 'Connect Slack' })).toBeEnabled()
  await page.screenshot(shot('settings-slack-not-connected'))
})

test('settings: connecting Slack', async ({ page }) => {
  await mockMe(page, 200, me(false))
  await mockGoogle(page, 200, googleNotConnected)
  // See 'login: redirecting to Slack': a 204 keeps the current document.
  await page.route('**/api/auth/login', (route) => route.fulfill({ status: 204 }))
  await page.goto('/settings')
  await page.getByRole('button', { name: 'Connect Slack' }).click({ noWaitAfter: true })
  await expect(page.getByRole('button', { name: 'Redirecting to Slack…' })).toBeDisabled()
  await page.screenshot(shot('settings-connecting-slack'))
})

test('settings: signing out', async ({ page }) => {
  await mockMe(page, 200, me(true))
  await mockGoogle(page, 200, googleNotConnected)
  await page.route('**/api/auth/logout', () => new Promise(() => {}))
  await page.goto('/settings')
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page.getByRole('button', { name: 'Signing out…' })).toBeDisabled()
  await page.screenshot(shot('settings-signing-out'))
})

test('settings: sign-out failed', async ({ page }) => {
  await mockMe(page, 200, me(true))
  await mockGoogle(page, 200, googleNotConnected)
  await page.route('**/api/auth/logout', (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"internal_error"}' }),
  )
  await page.goto('/settings')
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page.getByRole('alert')).toContainText('Could not sign out.')
  await page.screenshot(shot('settings-sign-out-failed'))
})

const googleRow = (page: Page) => page.getByRole('listitem', { name: 'Google Workspace' })

test('settings: Google Workspace status being checked', async ({ page }) => {
  await mockMe(page, 200, me(true))
  await page.route('**/api/integrations/google-workspace', () => new Promise(() => {}))
  await page.goto('/settings')
  await expect(googleRow(page).getByText('Checking…')).toBeVisible()
  await page.screenshot(shot('settings-google-checking'))
})

test('settings: Google Workspace status check failed', async ({ page }) => {
  await mockMe(page, 200, me(true))
  await mockGoogle(page, 500, { error: 'internal_error' })
  await page.goto('/settings')
  await expect(googleRow(page).getByRole('button', { name: 'Check Google Workspace again' })).toBeVisible()
  await page.screenshot(shot('settings-google-check-failed'))
})

test('settings: Google Workspace not available', async ({ page }) => {
  await settingsWithGoogle(page, { available: false, connected: false, email: '' })
  await page.goto('/settings')
  await expect(googleRow(page).getByText('Not available')).toBeVisible()
  await page.screenshot(shot('settings-google-unavailable'))
})

test('settings: Google Workspace not connected', async ({ page }) => {
  await settingsWithGoogle(page, googleNotConnected)
  await page.goto('/settings')
  await expect(page.getByRole('button', { name: 'Connect Google Workspace' })).toBeEnabled()
  await page.screenshot(shot('settings-google-not-connected'))
})

test('settings: connecting Google Workspace', async ({ page }) => {
  await settingsWithGoogle(page, googleNotConnected)
  // See 'login: redirecting to Slack': a 204 keeps the current document.
  await page.route('**/api/integrations/google-workspace/connect', (route) => route.fulfill({ status: 204 }))
  await page.goto('/settings')
  await page.getByRole('button', { name: 'Connect Google Workspace' }).click({ noWaitAfter: true })
  await expect(page.getByRole('button', { name: 'Redirecting to Google Workspace…' })).toBeDisabled()
  await page.screenshot(shot('settings-google-connecting'))
})

test('settings: Google Workspace connected', async ({ page }) => {
  await settingsWithGoogle(page, googleConnected)
  await page.goto('/settings')
  await expect(googleRow(page).getByText('Account: alice@example.com')).toBeVisible()
  await page.screenshot(shot('settings-google-connected'))
})

test('settings: disconnecting Google Workspace', async ({ page }) => {
  await settingsWithGoogle(page, googleConnected)
  await page.route('**/api/integrations/google-workspace/disconnect', () => new Promise(() => {}))
  await page.goto('/settings')
  await page.getByRole('button', { name: 'Disconnect Google Workspace' }).click()
  await expect(page.getByRole('button', { name: 'Disconnecting…' })).toBeDisabled()
  await page.screenshot(shot('settings-google-disconnecting'))
})

test('settings: disconnecting Google Workspace failed', async ({ page }) => {
  await settingsWithGoogle(page, googleConnected)
  await page.route('**/api/integrations/google-workspace/disconnect', (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"internal_error"}' }),
  )
  await page.goto('/settings')
  await page.getByRole('button', { name: 'Disconnect Google Workspace' }).click()
  await expect(googleRow(page).getByRole('alert')).toContainText('Could not disconnect Google Workspace.')
  await page.screenshot(shot('settings-google-disconnect-failed'))
})

for (const [result, name, text] of [
  ['connected', 'settings-google-notice-connected', 'Google Workspace is connected.'],
  ['access_denied', 'settings-google-notice-access-denied', 'you cancelled the request on Google'],
  ['missing_scope', 'settings-google-notice-missing-scope', 'you did not allow every requested permission'],
  ['failed', 'settings-google-notice-failed', 'Could not connect Google Workspace.'],
] as const) {
  test(`settings: Google Workspace connection result ${result}`, async ({ page }) => {
    await settingsWithGoogle(page, result === 'connected' ? googleConnected : googleNotConnected)
    await page.goto(`/settings?google_workspace=${result}`)
    await expect(page.getByText(text)).toBeVisible()
    await page.screenshot(shot(name))
  })
}
