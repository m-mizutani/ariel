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
  await page.goto('/settings')
  await expect(page.getByRole('listitem', { name: 'Slack' }).getByText('Connected')).toBeVisible()
  await page.screenshot(shot('settings-slack-connected'))
})

test('settings: Slack not connected', async ({ page }) => {
  await mockMe(page, 200, me(false))
  await page.goto('/settings')
  await expect(page.getByRole('button', { name: 'Connect Slack' })).toBeEnabled()
  await page.screenshot(shot('settings-slack-not-connected'))
})

test('settings: connecting Slack', async ({ page }) => {
  await mockMe(page, 200, me(false))
  // See 'login: redirecting to Slack': a 204 keeps the current document.
  await page.route('**/api/auth/login', (route) => route.fulfill({ status: 204 }))
  await page.goto('/settings')
  await page.getByRole('button', { name: 'Connect Slack' }).click({ noWaitAfter: true })
  await expect(page.getByRole('button', { name: 'Redirecting to Slack…' })).toBeDisabled()
  await page.screenshot(shot('settings-connecting-slack'))
})

test('settings: signing out', async ({ page }) => {
  await mockMe(page, 200, me(true))
  await page.route('**/api/auth/logout', () => new Promise(() => {}))
  await page.goto('/settings')
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page.getByRole('button', { name: 'Signing out…' })).toBeDisabled()
  await page.screenshot(shot('settings-signing-out'))
})

test('settings: sign-out failed', async ({ page }) => {
  await mockMe(page, 200, me(true))
  await page.route('**/api/auth/logout', (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"internal_error"}' }),
  )
  await page.goto('/settings')
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page.getByRole('alert')).toContainText('Could not sign out.')
  await page.screenshot(shot('settings-sign-out-failed'))
})
