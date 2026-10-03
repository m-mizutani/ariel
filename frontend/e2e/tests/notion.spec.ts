import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import { e2eNotionClientID, e2eUserID, fakeNotionURL } from '../../playwright.e2e.config'

// Notion connection use cases against the real server, which talks to
// e2e/fake-notion.mjs instead of Notion. The whole flow runs: the redirect to
// the authorization page, the callback, the token exchange, the stored
// connection, and the revocation on disconnection.
//
// The server keeps one in-memory store for every test and signs everyone in as
// the same user, so each test starts by disconnecting Notion.
//
// Not covered here, but by unit tests and screenshots: "Not available" (this
// server has Notion configured), "Reconnect required" (no page action makes
// Notion reject a refresh token), and "connected to another Robin user"
// (--no-auth has only one user).

const notionRow = (page: Page) => page.getByRole('listitem', { name: 'Notion' })

type FakeNotionRecords = { authorizes: Record<string, string>[]; revokes: string[] }

async function fakeNotion(request: APIRequestContext): Promise<FakeNotionRecords> {
  return (await (await request.get(`${fakeNotionURL}/__control`)).json()) as FakeNotionRecords
}

async function nextAuthorization(request: APIRequestContext, next: 'access_denied' | 'wrong_workspace') {
  await request.post(`${fakeNotionURL}/__control`, { data: { next } })
}

async function signIn(page: Page) {
  await page.goto('/login')
  await page.getByRole('button', { name: 'Sign in with Slack' }).click()
  await expect(page).toHaveURL('/settings')
  await expect(page.getByText(`Signed in as ${e2eUserID}`)).toBeVisible()
}

test.beforeEach(async ({ page, request }) => {
  await signIn(page)
  const res = await page.request.post('/api/v1/integrations/notion/disconnect')
  expect(res.status()).toBe(200)
  await request.delete(`${fakeNotionURL}/__control`)
  await page.reload()
  await expect(notionRow(page).getByText('Not connected')).toBeVisible()
})

test('a signed-in user sees Notion as not connected', async ({ page }) => {
  const notion = notionRow(page)
  await expect(notion.getByText(/read-only access to the Notion pages and databases you share with it/)).toBeVisible()
  await expect(notion.getByRole('button', { name: 'Connect Notion' })).toBeEnabled()
})

test('connecting Notion stores the connection and shows the account', async ({ page, request, baseURL }) => {
  await notionRow(page).getByRole('button', { name: 'Connect Notion' }).click()

  await expect(page.getByRole('status')).toHaveText('Notion is connected.')
  await expect(page).toHaveURL('/settings')
  const notion = notionRow(page)
  await expect(notion.getByText('Connected', { exact: true })).toBeVisible()
  await expect(notion.getByText('Account: E2E User (E2E Workspace)')).toBeVisible()
  await expect(notion.getByRole('button', { name: 'Disconnect Notion' })).toBeEnabled()

  const { authorizes } = await fakeNotion(request)
  expect(authorizes).toHaveLength(1)
  expect(authorizes[0].client_id).toBe(e2eNotionClientID)
  expect(authorizes[0].redirect_uri).toBe(`${baseURL}/api/v1/integrations/notion/callback`)
  expect(authorizes[0].response_type).toBe('code')
  expect(authorizes[0].owner).toBe('user')
  expect(authorizes[0].state).toBeTruthy()

  const status = await page.request.get('/api/v1/integrations/notion')
  expect(await status.json()).toEqual({
    available: true,
    connected: true,
    needs_reconnect: false,
    user_name: 'E2E User',
    workspace_name: 'E2E Workspace',
  })

  await page.reload()
  await expect(notion.getByText('Account: E2E User (E2E Workspace)')).toBeVisible()
  await expect(page.getByRole('status')).toHaveCount(0)
})

test('connecting again while connected changes nothing', async ({ page, request }) => {
  await notionRow(page).getByRole('button', { name: 'Connect Notion' }).click()
  await expect(page.getByRole('status')).toHaveText('Notion is connected.')

  await page.goto('/api/v1/integrations/notion/connect')

  await expect(page).toHaveURL('/settings')
  await expect(notionRow(page).getByText('Account: E2E User (E2E Workspace)')).toBeVisible()
  expect((await fakeNotion(request)).authorizes).toHaveLength(1)
})

test('disconnecting Notion revokes the token and removes the connection', async ({ page, request }) => {
  await notionRow(page).getByRole('button', { name: 'Connect Notion' }).click()
  await expect(page.getByRole('status')).toHaveText('Notion is connected.')

  await notionRow(page).getByRole('button', { name: 'Disconnect Notion' }).click()

  const notion = notionRow(page)
  await expect(notion.getByText('Not connected')).toBeVisible()
  await expect(notion.getByRole('button', { name: 'Connect Notion' })).toBeEnabled()
  await expect(notion.getByText(/Account:/)).toHaveCount(0)
  expect((await fakeNotion(request)).revokes).toHaveLength(1)

  await page.reload()
  await expect(notion.getByText('Not connected')).toBeVisible()
})

test('cancelling on Notion returns to the settings page with a message', async ({ page, context, request }) => {
  await nextAuthorization(request, 'access_denied')

  await notionRow(page).getByRole('button', { name: 'Connect Notion' }).click()

  await expect(page.getByRole('alert')).toHaveText(
    'Notion was not connected because you cancelled the request on Notion.',
  )
  await expect(page).toHaveURL('/settings')
  await expect(notionRow(page).getByText('Not connected')).toBeVisible()
  expect((await context.cookies()).map((c) => c.name)).not.toContain('robin_notion_oauth_state')
})

test('a workspace other than the configured one is rejected and its token revoked', async ({ page, request }) => {
  await nextAuthorization(request, 'wrong_workspace')

  await notionRow(page).getByRole('button', { name: 'Connect Notion' }).click()

  await expect(page.getByRole('alert')).toHaveText(
    'Notion was not connected because the workspace you chose is not the one Robin is set up for. Ask your Robin administrator which workspace to use.',
  )
  await expect(notionRow(page).getByText('Not connected')).toBeVisible()
  expect((await fakeNotion(request)).revokes).toHaveLength(1)
})

test('a callback that this browser did not start is rejected', async ({ page }) => {
  await page.goto('/api/v1/integrations/notion/callback?code=ok&state=forged-state')

  await expect(page.getByRole('alert')).toHaveText('Could not connect Notion. Try again.')
  await expect(page).toHaveURL('/settings')
  await expect(notionRow(page).getByText('Not connected')).toBeVisible()
})

test('a signed-out visitor cannot use the Notion endpoints', async ({ browser }) => {
  const page = await (await browser.newContext()).newPage()

  await page.goto('/api/v1/integrations/notion/connect')
  await expect(page).toHaveURL('/login')

  await page.goto('/api/v1/integrations/notion/callback?code=ok&state=s')
  await expect(page).toHaveURL('/login')

  const status = await page.request.get('/api/v1/integrations/notion')
  expect(status.status()).toBe(401)
  const disconnect = await page.request.post('/api/v1/integrations/notion/disconnect')
  expect(disconnect.status()).toBe(401)
  await page.context().close()
})
