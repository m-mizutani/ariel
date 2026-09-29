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

test('signing in shows the settings page and sets HttpOnly session cookies', async ({ page, context }) => {
  await signIn(page)

  // A user signed in without Slack has no stored token.
  await expect(page.getByRole('listitem', { name: 'Slack' }).getByText('Not connected')).toBeVisible()

  const cookies = await context.cookies()
  const names = cookies.map((c) => c.name)
  expect(names).toContain('ariel_session_id')
  expect(names).toContain('ariel_session_secret')
  expect(names).not.toContain('ariel_oauth_state')
  for (const c of cookies.filter((c) => c.name.startsWith('ariel_session_'))) {
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
  expect(names).not.toContain('ariel_session_id')

  await page.goto('/')
  await expect(page).toHaveURL('/login')
  const me = await page.request.get('/api/auth/me')
  expect(me.status()).toBe(401)
})

test('a callback without the matching state cookie fails and does not sign in', async ({ page }) => {
  await page.goto('/api/auth/callback?code=no-auth&state=forged')
  await expect(page).toHaveURL('/login?error=login_failed')
  await expect(page.getByRole('alert')).toContainText('Sign-in failed.')

  const me = await page.request.get('/api/auth/me')
  expect(me.status()).toBe(401)
})

test('a sign-in cancelled at Slack shows the cancellation message', async ({ page }) => {
  await page.goto('/api/auth/callback?error=access_denied&state=any')
  await expect(page).toHaveURL('/login?error=access_denied')
  await expect(page.getByRole('alert')).toContainText('Sign-in was cancelled.')
})
