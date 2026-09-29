import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AuthProvider } from '../contexts/auth-context'
import Settings from './Settings'

const startLogin = vi.fn()
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  startLogin: () => startLogin(),
}))

function me(connected: boolean) {
  return JSON.stringify({ team_id: 'T1', user_id: 'U1', name: 'Alice Example', slack_connected: connected })
}

function renderSettings() {
  return render(
    <MemoryRouter initialEntries={['/settings']}>
      <AuthProvider>
        <Routes>
          <Route path="/settings" element={<Settings />} />
          <Route path="/login" element={<p>login page</p>} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>,
  )
}

type Handler = (url: string, init?: RequestInit) => Promise<Response>

function stubFetch(handler: Handler) {
  const mock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => handler(String(input), init))
  vi.stubGlobal('fetch', mock)
  return mock
}

async function row(name: string) {
  return within(await screen.findByRole('listitem', { name }))
}

describe('Settings', () => {
  beforeEach(() => {
    startLogin.mockReset()
  })

  it('shows a linked Slack account without a way to disconnect it', async () => {
    stubFetch(async () => new Response(me(true), { status: 200 }))
    renderSettings()

    expect(await screen.findByRole('heading', { name: 'Settings', level: 1 })).toBeInTheDocument()
    expect(screen.getByText('Signed in as Alice Example')).toBeInTheDocument()
    const slack = await row('Slack')
    expect(slack.getByText('Connected')).toBeInTheDocument()
    expect(slack.getByText(/you cannot disconnect Slack on this page/)).toBeInTheDocument()
    expect(slack.queryByRole('button')).toBeNull()
    expect(screen.queryByRole('button', { name: /disconnect/i })).toBeNull()
  })

  it('offers to connect Slack when the account is not linked', async () => {
    stubFetch(async () => new Response(me(false), { status: 200 }))
    renderSettings()

    const slack = await row('Slack')
    expect(slack.getByText('Not connected')).toBeInTheDocument()
    expect(slack.getByRole('button', { name: 'Connect Slack' })).toBeEnabled()
  })

  it('starts the Slack authorization and disables the button', async () => {
    stubFetch(async () => new Response(me(false), { status: 200 }))
    renderSettings()

    fireEvent.click((await row('Slack')).getByRole('button', { name: 'Connect Slack' }))

    expect(startLogin).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('button', { name: 'Redirecting to Slack…' })).toBeDisabled()
  })

  it.each(['Google Workspace', 'Notion', 'GitHub'])('shows %s as coming soon with a disabled button', async (name) => {
    const fetchMock = stubFetch(async () => new Response(me(true), { status: 200 }))
    renderSettings()

    const service = await row(name)
    expect(service.getByText('Coming soon')).toBeInTheDocument()
    const button = service.getByRole('button', { name: `Connect ${name}` })
    expect(button).toBeDisabled()

    fireEvent.click(button)
    expect(startLogin).not.toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('lists the services in display order', async () => {
    stubFetch(async () => new Response(me(true), { status: 200 }))
    renderSettings()

    await screen.findByRole('heading', { name: 'Integrations' })
    expect(screen.getAllByRole('heading', { level: 3 }).map((h) => h.textContent)).toEqual([
      'Slack',
      'Google Workspace',
      'Notion',
      'GitHub',
    ])
  })

  it('signs out and moves to the login page', async () => {
    let signedOut = false
    const fetchMock = stubFetch(async (url, init) => {
      if (url === '/api/auth/logout' && init?.method === 'POST') {
        signedOut = true
        return new Response('{"success":true}', { status: 200 })
      }
      return signedOut
        ? new Response('{"error":"unauthenticated"}', { status: 401 })
        : new Response(me(true), { status: 200 })
    })
    renderSettings()

    fireEvent.click(await screen.findByRole('button', { name: 'Sign out' }))

    expect(await screen.findByText('login page')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith('/api/auth/logout', expect.objectContaining({ method: 'POST' }))
  })

  it('reports a failed sign-out and enables the button again', async () => {
    stubFetch(async (url) =>
      url === '/api/auth/logout'
        ? new Response('{"error":"internal_error"}', { status: 500 })
        : new Response(me(true), { status: 200 }),
    )
    renderSettings()

    fireEvent.click(await screen.findByRole('button', { name: 'Sign out' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Could not sign out. Try again.')
    await waitFor(() => expect(screen.getByRole('button', { name: 'Sign out' })).toBeEnabled())
  })
})
