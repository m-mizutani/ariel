import { expect, test, type Page } from '@playwright/test'
import { e2eGitHubClientID, e2eUserID } from '../../playwright.e2e.config'

// GitHub connection use cases against the real server, started with a GitHub
// App that does not exist. The browser is stopped before it reaches
// github.com, so these tests cover everything Ariel does before and after
// GitHub: starting the authorization, the state and PKCE cookies, and every
// callback that does not need a real authorization code. The connected state
// needs GitHub and is covered by unit tests and screenshots.

const githubRow = (page: Page) => page.getByRole('listitem', { name: 'GitHub' })

async function signIn(page: Page) {
  await page.goto('/login')
  await page.getByRole('button', { name: 'Sign in with Slack' }).click()
  await expect(page).toHaveURL('/settings')
  await expect(page.getByText(`Signed in as ${e2eUserID}`)).toBeVisible()
}

type ConnectResponse = { location: URL; setCookies: string[] }

// Lets the connect request reach the real server but stops the browser before
// it follows the redirect to GitHub (a route handler is not called for a
// redirect target, so GitHub itself cannot be intercepted). The server sets
// two cookies; each is copied into the browser with the path and attributes
// the server gave it, so the callback clears them as it would in a real flow.
// The returned function gives the redirect and the raw Set-Cookie headers.
async function stopBeforeGitHub(page: Page): Promise<() => ConnectResponse | undefined> {
  let result: ConnectResponse | undefined
  await page.route('**/api/v1/integrations/github/connect', async (route) => {
    const response = await route.fetch({ maxRedirects: 0 })
    expect(response.status()).toBe(302)
    const setCookies = response
      .headersArray()
      .filter((h) => h.name.toLowerCase() === 'set-cookie')
      .map((h) => h.value)
    result = { location: new URL(response.headers()['location']), setCookies }
    const host = new URL(route.request().url()).hostname
    await page.context().addCookies(
      setCookies.map((header) => {
        const [pair] = header.split(';')
        const separator = pair.indexOf('=')
        return {
          name: pair.slice(0, separator),
          value: pair.slice(separator + 1),
          domain: host,
          path: '/api/v1/integrations/github',
          httpOnly: true,
          sameSite: 'Lax' as const,
        }
      }),
    )
    await route.fulfill({
      status: 200,
      contentType: 'text/html',
      body: '<title>Redirect to GitHub stopped by the test</title>',
    })
  })
  return () => result
}

test('a signed-in user sees GitHub as not connected', async ({ page }) => {
  await signIn(page)

  const github = githubRow(page)
  await expect(github.getByText('Not connected')).toBeVisible()
  await expect(github.getByText(/read-only access to the repositories, issues, pull requests/)).toBeVisible()
  await expect(github.getByRole('button', { name: 'Connect GitHub' })).toBeEnabled()
})

test('connecting sends the browser to GitHub with the configured app and a PKCE challenge', async ({
  page,
  baseURL,
}) => {
  await signIn(page)
  const connect = await stopBeforeGitHub(page)

  await githubRow(page).getByRole('button', { name: 'Connect GitHub' }).click()
  await expect(page).toHaveURL('/api/v1/integrations/github/connect')

  const result = connect()
  expect(result).toBeDefined()
  const { location, setCookies } = result!
  expect(location.origin).toBe('https://github.com')
  expect(location.pathname).toBe('/login/oauth/authorize')
  const q = location.searchParams
  expect(q.get('client_id')).toBe(e2eGitHubClientID)
  expect(q.get('redirect_uri')).toBe(`${baseURL}/api/v1/integrations/github/callback`)
  expect(q.get('response_type')).toBe('code')
  expect(q.get('code_challenge_method')).toBe('S256')
  expect(q.get('code_challenge')).toMatch(/^[A-Za-z0-9_-]{43}$/)
  expect(q.get('allow_signup')).toBe('false')
  expect(q.get('state')).toBeTruthy()

  const state = setCookies.find((c) => c.startsWith('ariel_github_oauth_state='))
  const verifier = setCookies.find((c) => c.startsWith('ariel_github_oauth_verifier='))
  for (const cookie of [state, verifier]) {
    expect(cookie).toBeDefined()
    expect(cookie).toContain('Path=/api/v1/integrations/github')
    expect(cookie).toContain('HttpOnly')
    expect(cookie).toContain('SameSite=Lax')
    expect(cookie).toContain('Max-Age=600')
  }
  expect(state!.startsWith(`ariel_github_oauth_state=${q.get('state')}.`)).toBe(true)
  expect(verifier).toMatch(/^ariel_github_oauth_verifier=[A-Za-z0-9_-]{43};/)
})

test('cancelling on GitHub returns to the settings page with a message', async ({ page, context }) => {
  await signIn(page)
  const connect = await stopBeforeGitHub(page)
  await githubRow(page).getByRole('button', { name: 'Connect GitHub' }).click()
  await expect(page).toHaveURL('/api/v1/integrations/github/connect')
  const state = connect()!.location.searchParams.get('state')
  const names = (await context.cookies()).map((c) => c.name)
  expect(names).toContain('ariel_github_oauth_state')
  expect(names).toContain('ariel_github_oauth_verifier')

  await page.goto(`/api/v1/integrations/github/callback?error=access_denied&state=${state}`)

  await expect(page.getByRole('alert')).toHaveText('GitHub was not connected because you cancelled the request on GitHub.')
  await expect(page).toHaveURL('/settings')
  await expect(githubRow(page).getByText('Not connected')).toBeVisible()
  const remaining = (await context.cookies()).map((c) => c.name)
  expect(remaining).not.toContain('ariel_github_oauth_state')
  expect(remaining).not.toContain('ariel_github_oauth_verifier')
})

test('a callback that this browser did not start is rejected', async ({ page }) => {
  await signIn(page)

  await page.goto('/api/v1/integrations/github/callback?code=forged-code&state=forged-state')

  await expect(page.getByRole('alert')).toHaveText('Could not connect GitHub. Try again.')
  await expect(page).toHaveURL('/settings')
  await expect(githubRow(page).getByText('Not connected')).toBeVisible()
})

test('a signed-out visitor cannot use the GitHub endpoints', async ({ page }) => {
  await page.goto('/api/v1/integrations/github/connect')
  await expect(page).toHaveURL('/login')

  await page.goto('/api/v1/integrations/github/callback?code=c&state=s')
  await expect(page).toHaveURL('/login')

  const status = await page.request.get('/api/v1/integrations/github')
  expect(status.status()).toBe(401)
  const disconnect = await page.request.post('/api/v1/integrations/github/disconnect')
  expect(disconnect.status()).toBe(401)
})

test('disconnecting an account that is not connected succeeds and changes nothing', async ({ page }) => {
  await signIn(page)

  const res = await page.request.post('/api/v1/integrations/github/disconnect')
  expect(res.status()).toBe(200)
  expect(await res.json()).toEqual({ success: true })

  const status = await page.request.get('/api/v1/integrations/github')
  expect(await status.json()).toEqual({ available: true, connected: false, login: '' })
  await page.reload()
  await expect(githubRow(page).getByText('Not connected')).toBeVisible()
})

test('the API paths without the version are gone', async ({ page }) => {
  await signIn(page)
  for (const path of ['/api/auth/me', '/api/integrations/google-workspace', '/api/integrations/github']) {
    const res = await page.request.get(path)
    expect(res.status()).toBe(404)
    expect(await res.json()).toEqual({ error: 'not_found' })
  }
})
