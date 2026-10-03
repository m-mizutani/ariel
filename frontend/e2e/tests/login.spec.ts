import { expect, test, type Page } from '@playwright/test'
import { e2eUserID } from '../../playwright.e2e.config'

// Sign-in use cases on the web UI, against the real server running with
// --no-auth (every sign-in becomes e2eUserID; no Slack token is stored).

async function signIn(page: Page) {
  await page.goto('/login')
  await page.getByRole('button', { name: 'Sign in with Slack' }).click()
  await expect(page).toHaveURL('/settings')
  await expect(page.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible()
  await expect(page.getByText(`Signed in as ${e2eUserID}`)).toBeVisible()
}

test('a signed-out visitor is sent to the login page', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL('/login')
  await expect(page.getByRole('button', { name: 'Sign in with Slack' })).toBeVisible()
})

test('the login page shows the logo and the page has the favicon', async ({ page }) => {
  await page.goto('/login')
  const logo = page.getByRole('img', { name: 'Robin logo' })
  await expect(logo).toBeVisible()
  // naturalWidth is 0 when the browser could not load or decode the image.
  expect(await logo.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBeGreaterThan(0)

  const href = await page.locator('link[rel="icon"]').getAttribute('href')
  expect(href).toBe('/favicon.png')
  const favicon = await page.request.get(href!)
  expect(favicon.status()).toBe(200)
  expect(favicon.headers()['content-type']).toBe('image/png')
})

test('signing in shows the settings page and sets HttpOnly session cookies', async ({ page, context }) => {
  await signIn(page)

  // A user signed in without Slack has no stored token.
  await expect(page.getByRole('listitem', { name: 'Slack' }).getByText('Not connected')).toBeVisible()

  const cookies = await context.cookies()
  const names = cookies.map((c) => c.name)
  expect(names).toContain('robin_session_id')
  expect(names).toContain('robin_session_secret')
  expect(names).not.toContain('robin_oauth_state')
  for (const c of cookies.filter((c) => c.name.startsWith('robin_session_'))) {
    expect(c.httpOnly).toBe(true)
    expect(c.sameSite).toBe('Lax')
  }
})

test('the session survives a reload', async ({ page }) => {
  await signIn(page)
  await page.reload()
  await expect(page.getByText(`Signed in as ${e2eUserID}`)).toBeVisible()
})

test('a signed-in user opening the login page is sent to the settings page', async ({ page }) => {
  await signIn(page)
  await page.goto('/login')
  await expect(page).toHaveURL('/settings')
})

test('signing out ends the session', async ({ page, context }) => {
  await signIn(page)
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page).toHaveURL('/login')

  const names = (await context.cookies()).map((c) => c.name)
  expect(names).not.toContain('robin_session_id')

  await page.goto('/')
  await expect(page).toHaveURL('/login')
  const me = await page.request.get('/api/v1/auth/me')
  expect(me.status()).toBe(401)
})

test('a callback without the matching state cookie fails and does not sign in', async ({ page }) => {
  await page.goto('/api/v1/auth/callback?code=no-auth&state=forged')
  await expect(page).toHaveURL('/login?error=login_failed')
  await expect(page.getByRole('alert')).toContainText('Sign-in failed.')

  const me = await page.request.get('/api/v1/auth/me')
  expect(me.status()).toBe(401)
})

test('a sign-in cancelled at Slack shows the cancellation message', async ({ page }) => {
  await page.goto('/api/v1/auth/callback?error=access_denied&state=any')
  await expect(page).toHaveURL('/login?error=access_denied')
  await expect(page.getByRole('alert')).toContainText('Sign-in was cancelled.')
})
