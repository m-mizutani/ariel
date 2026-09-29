import { expect, test, type Page } from '@playwright/test'
import { e2eGoogleClientID, e2eUserID } from '../../playwright.e2e.config'

// Google Workspace connection use cases against the real server, started with
// a Google OAuth client that does not exist. The browser is stopped before it
// reaches accounts.google.com, so these tests cover everything Ariel does before and
// after Google: starting the authorization, the state cookie, and every
// callback that does not need a real authorization code. The connected state
// needs Google and is covered by unit tests and screenshots.

const googleRow = (page: Page) => page.getByRole('listitem', { name: 'Google Workspace' })

async function signIn(page: Page) {
  await page.goto('/login')
  await page.getByRole('button', { name: 'Sign in with Slack' }).click()
  await expect(page).toHaveURL('/settings')
  await expect(page.getByText(`Signed in as ${e2eUserID}`)).toBeVisible()
}

// Lets the connect request reach the real server but stops the browser before
// it follows the redirect to Google (a route handler is not called for a
// redirect target, so Google itself cannot be intercepted). The server's
// Set-Cookie is passed on to the browser, and the returned function gives the
// authorization URL the server redirected to.
async function stopBeforeGoogle(page: Page): Promise<() => URL | undefined> {
  let location: URL | undefined
  await page.route('**/api/integrations/google-workspace/connect', async (route) => {
    const response = await route.fetch({ maxRedirects: 0 })
    expect(response.status()).toBe(302)
    location = new URL(response.headers()['location'])
    await route.fulfill({
      status: 200,
      contentType: 'text/html',
      headers: { 'set-cookie': response.headers()['set-cookie'] },
      body: '<title>Redirect to Google stopped by the test</title>',
    })
  })
  return () => location
}

test('a signed-in user sees Google Workspace as not connected', async ({ page }) => {
  await signIn(page)

  const google = googleRow(page)
  await expect(google.getByText('Not connected')).toBeVisible()
  await expect(google.getByText(/read-only access to your Google Calendar, Gmail, and Drive files/)).toBeVisible()
  await expect(google.getByRole('button', { name: 'Connect Google Workspace' })).toBeEnabled()
})

test('connecting sends the browser to Google with the configured client and scopes', async ({ page, context, baseURL }) => {
  await signIn(page)
  const googleURL = await stopBeforeGoogle(page)

  await googleRow(page).getByRole('button', { name: 'Connect Google Workspace' }).click()
  await expect(page).toHaveURL('/api/integrations/google-workspace/connect')

  const url = googleURL()
  expect(url).toBeDefined()
  expect(url!.origin).toBe('https://accounts.google.com')
  expect(url!.pathname).toBe('/o/oauth2/v2/auth')
  const q = url!.searchParams
  expect(q.get('client_id')).toBe(e2eGoogleClientID)
  expect(q.get('redirect_uri')).toBe(`${baseURL}/api/integrations/google-workspace/callback`)
  expect(q.get('response_type')).toBe('code')
  expect(q.get('scope')?.split(' ')).toEqual([
    'openid',
    'email',
    'https://www.googleapis.com/auth/calendar.readonly',
    'https://www.googleapis.com/auth/drive.readonly',
    'https://www.googleapis.com/auth/gmail.readonly',
  ])
  expect(q.get('access_type')).toBe('offline')
  expect(q.get('prompt')).toBe('consent')
  expect(q.get('state')).toBeTruthy()

  const cookie = (await context.cookies()).find((c) => c.name === 'ariel_google_oauth_state')
  expect(cookie).toBeDefined()
  expect(cookie!.httpOnly).toBe(true)
  expect(cookie!.sameSite).toBe('Lax')
  expect(cookie!.path).toBe('/api/integrations/google-workspace')
  expect(cookie!.value.startsWith(`${q.get('state')}.`)).toBe(true)
})

test('cancelling on Google returns to the settings page with a message', async ({ page, context }) => {
  await signIn(page)
  const googleURL = await stopBeforeGoogle(page)
  await googleRow(page).getByRole('button', { name: 'Connect Google Workspace' }).click()
  await expect(page).toHaveURL('/api/integrations/google-workspace/connect')
  const state = googleURL()!.searchParams.get('state')
  expect((await context.cookies()).map((c) => c.name)).toContain('ariel_google_oauth_state')

  await page.goto(`/api/integrations/google-workspace/callback?error=access_denied&state=${state}`)

  await expect(page.getByRole('alert')).toHaveText(
    'Google Workspace was not connected because you cancelled the request on Google.',
  )
  await expect(page).toHaveURL('/settings')
  await expect(googleRow(page).getByText('Not connected')).toBeVisible()
  const names = (await context.cookies()).map((c) => c.name)
  expect(names).not.toContain('ariel_google_oauth_state')
})

test('a callback that this browser did not start is rejected', async ({ page }) => {
  await signIn(page)

  await page.goto('/api/integrations/google-workspace/callback?code=forged-code&state=forged-state')

  await expect(page.getByRole('alert')).toHaveText('Could not connect Google Workspace. Try again.')
  await expect(page).toHaveURL('/settings')
  await expect(googleRow(page).getByText('Not connected')).toBeVisible()
})

test('a signed-out visitor cannot use the Google Workspace endpoints', async ({ page }) => {
  await page.goto('/api/integrations/google-workspace/connect')
  await expect(page).toHaveURL('/login')

  await page.goto('/api/integrations/google-workspace/callback?code=c&state=s')
  await expect(page).toHaveURL('/login')

  const status = await page.request.get('/api/integrations/google-workspace')
  expect(status.status()).toBe(401)
  const disconnect = await page.request.post('/api/integrations/google-workspace/disconnect')
  expect(disconnect.status()).toBe(401)
})

test('disconnecting an account that is not connected succeeds and changes nothing', async ({ page }) => {
  await signIn(page)

  const res = await page.request.post('/api/integrations/google-workspace/disconnect')
  expect(res.status()).toBe(200)
  expect(await res.json()).toEqual({ success: true })

  const status = await page.request.get('/api/integrations/google-workspace')
  expect(await status.json()).toEqual({ available: true, connected: false, email: '' })
  await page.reload()
  await expect(googleRow(page).getByText('Not connected')).toBeVisible()
})
