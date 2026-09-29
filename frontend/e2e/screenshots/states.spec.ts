import { expect, test, type Page } from '@playwright/test'

// One test per screen state listed in the spec. Each test mocks the API the
// state depends on, waits for the state to be visible, and saves
// screenshots/<name>.png.

// Resolved against the working directory, which is frontend/ for `pnpm screenshots`.
const shot = (name: string) => ({ path: `screenshots/${name}.png`, fullPage: true })

const me = (connected: boolean) =>
  JSON.stringify({ team_id: 'T0123ABCD', user_id: 'U0123ABCD', name: 'Alice Example', slack_connected: connected })

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
  // Keep the navigation to /api/auth/login pending so the page stays on the
  // redirecting state.
  await page.route('**/api/auth/login', () => new Promise(() => {}))
  await page.goto('/login')
  await page.getByRole('button', { name: 'Sign in with Slack' }).click()
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

test('home: Slack account linked', async ({ page }) => {
  await mockMe(page, 200, me(true))
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Signed in as Alice Example' })).toBeVisible()
  await page.screenshot(shot('home-linked'))
})

test('home: Slack account not linked', async ({ page }) => {
  await mockMe(page, 200, me(false))
  await page.goto('/')
  await expect(page.getByRole('alert')).toContainText('not linked')
  await page.screenshot(shot('home-not-linked'))
})

test('home: signing out', async ({ page }) => {
  await mockMe(page, 200, me(true))
  await page.route('**/api/auth/logout', () => new Promise(() => {}))
  await page.goto('/')
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page.getByRole('button', { name: 'Signing out…' })).toBeDisabled()
  await page.screenshot(shot('home-signing-out'))
})

test('home: sign-out failed', async ({ page }) => {
  await mockMe(page, 200, me(true))
  await page.route('**/api/auth/logout', (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"internal_error"}' }),
  )
  await page.goto('/')
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page.getByRole('alert')).toContainText('Could not sign out.')
  await page.screenshot(shot('home-sign-out-failed'))
})
