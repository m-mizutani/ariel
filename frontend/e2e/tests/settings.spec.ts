import { expect, test, type Page } from '@playwright/test'
import { e2eUserID } from '../../playwright.e2e.config'

// Settings page use cases against the real server running with --no-auth.
// A --no-auth sign-in stores no Slack token, so Slack is always shown as not
// connected here; the connected state is covered by unit tests and screenshots.

async function signIn(page: Page) {
  await page.goto('/login')
  await page.getByRole('button', { name: 'Sign in with Slack' }).click()
  await expect(page).toHaveURL('/settings')
  await expect(page.getByText(`Signed in as ${e2eUserID}`)).toBeVisible()
}

test('the settings page lists Slack and the services coming later', async ({ page }) => {
  await signIn(page)

  await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Slack', 'Google Workspace', 'Notion', 'GitHub'])

  const slack = page.getByRole('listitem', { name: 'Slack' })
  await expect(slack.getByText('Not connected')).toBeVisible()
  await expect(slack.getByRole('button', { name: 'Connect Slack' })).toBeEnabled()

  for (const name of ['Google Workspace', 'Notion', 'GitHub']) {
    const service = page.getByRole('listitem', { name })
    await expect(service.getByText('Coming soon')).toBeVisible()
    await expect(service.getByRole('button', { name: `Connect ${name}` })).toBeDisabled()
  }
})

test('the root and unknown paths lead a signed-in user to the settings page', async ({ page }) => {
  await signIn(page)
  for (const path of ['/', '/no-such-page']) {
    await page.goto(path)
    await expect(page).toHaveURL('/settings')
    await expect(page.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible()
  }
})

test('connecting Slack runs the sign-in again and returns to the settings page', async ({ page }) => {
  await signIn(page)
  await page.getByRole('button', { name: 'Connect Slack' }).click()
  await expect(page).toHaveURL('/settings')
  await expect(page.getByText(`Signed in as ${e2eUserID}`)).toBeVisible()
})

test('a signed-out visitor opening the settings page is sent to the login page', async ({ page }) => {
  await page.goto('/settings')
  await expect(page).toHaveURL('/login')
  await expect(page.getByText(/Signed in as/)).toHaveCount(0)

  const me = await page.request.get('/api/auth/me')
  expect(me.status()).toBe(401)
})

test('after signing out, the settings page is no longer available', async ({ page }) => {
  await signIn(page)
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page).toHaveURL('/login')

  await page.goto('/settings')
  await expect(page).toHaveURL('/login')
  const me = await page.request.get('/api/auth/me')
  expect(me.status()).toBe(401)
})
