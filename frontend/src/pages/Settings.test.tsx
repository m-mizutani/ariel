import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AuthProvider } from '../contexts/auth-context'
import Settings from './Settings'

const startLogin = vi.fn()
const startGoogleWorkspaceConnect = vi.fn()
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  startLogin: () => startLogin(),
  startGoogleWorkspaceConnect: () => startGoogleWorkspaceConnect(),
}))

function me(connected: boolean) {
  return JSON.stringify({ team_id: 'T1', user_id: 'U1', name: 'Alice Example', slack_connected: connected })
}

function googleStatus(status: { available: boolean; connected: boolean; email: string }) {
  return JSON.stringify(status)
}

const googleNotConnected = googleStatus({ available: true, connected: false, email: '' })
const googleConnected = googleStatus({ available: true, connected: true, email: 'alice@example.com' })

const googlePath = '/api/integrations/google-workspace'

// Shows the current URL, so tests can check that the result parameter is
// removed after the notice is shown.
function CurrentLocation() {
  const location = useLocation()
  return <p data-testid="location">{location.pathname + location.search}</p>
}

function renderSettings(path = '/settings') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <Routes>
          <Route
            path="/settings"
            element={
              <>
                <Settings />
                <CurrentLocation />
              </>
            }
          />
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

// Answers /api/auth/me with meBody and the Google Workspace status with
// googleBody; other paths go to rest.
function stubApi(meBody: string, googleBody: string = googleNotConnected, rest?: Handler) {
  return stubFetch(async (url, init) => {
    if (url === '/api/auth/me') {
      return new Response(meBody, { status: 200 })
    }
    if (url === googlePath) {
      return new Response(googleBody, { status: 200 })
    }
    if (rest) {
      return rest(url, init)
    }
    return new Response('{"error":"not_found"}', { status: 404 })
  })
}

function callsTo(mock: ReturnType<typeof stubFetch>, url: string, method = 'GET') {
  return mock.mock.calls.filter(([input, init]) => String(input) === url && (init?.method ?? 'GET') === method)
}

async function row(name: string) {
  return within(await screen.findByRole('listitem', { name }))
}

describe('Settings', () => {
  beforeEach(() => {
    startLogin.mockReset()
    startGoogleWorkspaceConnect.mockReset()
  })

  it('shows a linked Slack account without a way to disconnect it', async () => {
    stubApi(me(true))
    renderSettings()

    expect(await screen.findByRole('heading', { name: 'Settings', level: 1 })).toBeInTheDocument()
    expect(screen.getByText('Signed in as Alice Example')).toBeInTheDocument()
    const slack = await row('Slack')
    expect(slack.getByText('Connected')).toBeInTheDocument()
    expect(slack.getByText(/you cannot disconnect Slack on this page/)).toBeInTheDocument()
    expect(slack.queryByRole('button')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Disconnect Slack' })).toBeNull()
  })

  it('offers to connect Slack when the account is not linked', async () => {
    stubApi(me(false))
    renderSettings()

    const slack = await row('Slack')
    expect(slack.getByText('Not connected')).toBeInTheDocument()
    expect(slack.getByRole('button', { name: 'Connect Slack' })).toBeEnabled()
  })

  it('starts the Slack authorization and disables the button', async () => {
    stubApi(me(false))
    renderSettings()

    fireEvent.click((await row('Slack')).getByRole('button', { name: 'Connect Slack' }))

    expect(startLogin).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('button', { name: 'Redirecting to Slack…' })).toBeDisabled()
  })

  it.each(['Notion', 'GitHub'])('shows %s as coming soon with a disabled button', async (name) => {
    const fetchMock = stubApi(me(true))
    renderSettings()

    const service = await row(name)
    expect(service.getByText('Coming soon')).toBeInTheDocument()
    const button = service.getByRole('button', { name: `Connect ${name}` })
    expect(button).toBeDisabled()

    fireEvent.click(button)
    expect(startLogin).not.toHaveBeenCalled()
    expect(startGoogleWorkspaceConnect).not.toHaveBeenCalled()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
  })

  it('lists the services in display order', async () => {
    stubApi(me(true))
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
      if (url === googlePath) {
        return new Response(googleNotConnected, { status: 200 })
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
    stubApi(me(true), googleNotConnected, async () => new Response('{"error":"internal_error"}', { status: 500 }))
    renderSettings()

    fireEvent.click(await screen.findByRole('button', { name: 'Sign out' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Could not sign out. Try again.')
    await waitFor(() => expect(screen.getByRole('button', { name: 'Sign out' })).toBeEnabled())
  })
})

describe('Settings: Google Workspace', () => {
  beforeEach(() => {
    startLogin.mockReset()
    startGoogleWorkspaceConnect.mockReset()
  })

  it('shows that the status is being checked', async () => {
    stubFetch(async (url) =>
      url === googlePath ? new Promise<Response>(() => {}) : new Response(me(true), { status: 200 }),
    )
    renderSettings()

    const google = await row('Google Workspace')
    expect(google.getByText('Checking…')).toBeInTheDocument()
    expect(google.queryByRole('button')).toBeNull()
  })

  it('reports a failed status check and checks again', async () => {
    let failing = true
    const fetchMock = stubFetch(async (url) => {
      if (url === googlePath) {
        return failing
          ? new Response('{"error":"internal_error"}', { status: 500 })
          : new Response(googleNotConnected, { status: 200 })
      }
      return new Response(me(true), { status: 200 })
    })
    renderSettings()

    const google = await row('Google Workspace')
    expect(await google.findByText('Could not load the connection status.')).toBeInTheDocument()

    failing = false
    fireEvent.click(google.getByRole('button', { name: 'Check Google Workspace again' }))

    expect(await google.findByText('Not connected')).toBeInTheDocument()
    expect(callsTo(fetchMock, googlePath)).toHaveLength(2)
  })

  it('shows that the server has not set up the integration', async () => {
    stubApi(me(true), googleStatus({ available: false, connected: false, email: '' }))
    renderSettings()

    const google = await row('Google Workspace')
    expect(await google.findByText('Not available')).toBeInTheDocument()
    expect(google.getByText('Your Ariel administrator has not set up this integration.')).toBeInTheDocument()
    expect(google.queryByRole('button')).toBeNull()
  })

  it('starts the Google authorization and disables the button', async () => {
    stubApi(me(true))
    renderSettings()

    const google = await row('Google Workspace')
    expect(await google.findByText('Not connected')).toBeInTheDocument()
    expect(google.getByText(/read-only access to your Google Calendar, Gmail, and Drive files/)).toBeInTheDocument()

    fireEvent.click(google.getByRole('button', { name: 'Connect Google Workspace' }))

    expect(startGoogleWorkspaceConnect).toHaveBeenCalledTimes(1)
    expect(startLogin).not.toHaveBeenCalled()
    expect(google.getByRole('button', { name: 'Redirecting to Google Workspace…' })).toBeDisabled()
  })

  it('shows the connected account with a way to disconnect it', async () => {
    stubApi(me(true), googleConnected)
    renderSettings()

    const google = await row('Google Workspace')
    expect(await google.findByText('Connected')).toBeInTheDocument()
    expect(google.getByText('Account: alice@example.com')).toBeInTheDocument()
    expect(google.getByRole('button', { name: 'Disconnect Google Workspace' })).toBeEnabled()
    expect((await row('Slack')).queryByRole('button')).toBeNull()
  })

  it('disconnects and shows the account as not connected', async () => {
    let connected = true
    let release: () => void = () => {}
    const fetchMock = stubFetch(async (url, init) => {
      if (url === `${googlePath}/disconnect` && init?.method === 'POST') {
        await new Promise<void>((resolve) => {
          release = resolve
        })
        connected = false
        return new Response('{"success":true}', { status: 200 })
      }
      if (url === googlePath) {
        return new Response(connected ? googleConnected : googleNotConnected, { status: 200 })
      }
      return new Response(me(true), { status: 200 })
    })
    renderSettings()

    const google = await row('Google Workspace')
    fireEvent.click(await google.findByRole('button', { name: 'Disconnect Google Workspace' }))

    expect(await google.findByRole('button', { name: 'Disconnecting…' })).toBeDisabled()
    release()

    expect(await google.findByText('Not connected')).toBeInTheDocument()
    expect(google.getByRole('button', { name: 'Connect Google Workspace' })).toBeEnabled()
    expect(google.queryByText(/Account:/)).toBeNull()
    expect(callsTo(fetchMock, `${googlePath}/disconnect`, 'POST')).toHaveLength(1)
    expect(callsTo(fetchMock, googlePath)).toHaveLength(2)
  })

  it('reports a failed disconnection and keeps the account connected', async () => {
    stubApi(me(true), googleConnected, async () => new Response('{"error":"internal_error"}', { status: 500 }))
    renderSettings()

    const google = await row('Google Workspace')
    fireEvent.click(await google.findByRole('button', { name: 'Disconnect Google Workspace' }))

    expect(await google.findByRole('alert')).toHaveTextContent('Could not disconnect Google Workspace. Try again.')
    expect(google.getByText('Connected')).toBeInTheDocument()
    await waitFor(() => expect(google.getByRole('button', { name: 'Disconnect Google Workspace' })).toBeEnabled())
  })

  it.each([
    ['connected', 'status', 'Google Workspace is connected.'],
    ['access_denied', 'alert', 'Google Workspace was not connected because you cancelled the request on Google.'],
    [
      'missing_scope',
      'alert',
      'Google Workspace was not connected because you did not allow every requested permission. Connect again and allow all of them.',
    ],
    [
      'account_in_use',
      'alert',
      'Google Workspace was not connected because this Google account is already connected to another Ariel user. Connect a different Google account.',
    ],
    ['failed', 'alert', 'Could not connect Google Workspace. Try again.'],
  ])('shows the result %s and removes it from the URL', async (result, role, text) => {
    stubApi(me(true), result === 'connected' ? googleConnected : googleNotConnected)
    renderSettings(`/settings?google_workspace=${result}`)

    expect(await screen.findByRole(role, { name: '' })).toHaveTextContent(text)
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent(/^\/settings$/))
  })

  it('ignores an unknown result', async () => {
    stubApi(me(true))
    renderSettings('/settings?google_workspace=unknown')

    await screen.findByRole('heading', { name: 'Integrations' })
    expect(screen.queryByRole('status')).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
  })
})
