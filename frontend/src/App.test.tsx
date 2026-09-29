import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it, vi } from 'vitest'
import App from './App'
import { AuthProvider } from './contexts/auth-context'

function renderApp(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <App />
      </AuthProvider>
    </MemoryRouter>,
  )
}

const meBody = JSON.stringify({ team_id: 'T1', user_id: 'U1', name: 'Alice', slack_connected: true })

describe('App routing', () => {
  it.each(['/', '/settings', '/no-such-page'])('shows the settings page to a signed-in user at %s', async (path) => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(meBody, { status: 200 })))
    renderApp(path)
    expect(await screen.findByRole('heading', { name: 'Settings', level: 1 })).toBeInTheDocument()
  })

  it.each(['/', '/settings'])('sends a signed-out visitor at %s to the login page', async (path) => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"error":"unauthenticated"}', { status: 401 })))
    renderApp(path)
    expect(await screen.findByRole('button', { name: 'Sign in with Slack' })).toBeInTheDocument()
    expect(screen.queryByText(/Signed in as/)).toBeNull()
  })
})
