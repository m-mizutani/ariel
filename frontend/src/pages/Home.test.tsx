import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { describe, expect, it, vi } from 'vitest'
import { AuthProvider } from '../contexts/auth-context'
import Home from './Home'

function me(connected: boolean) {
  return JSON.stringify({ team_id: 'T1', user_id: 'U1', name: 'Alice Example', slack_connected: connected })
}

function renderHome() {
  return render(
    <MemoryRouter initialEntries={['/']}>
      <AuthProvider>
        <Routes>
          <Route path="/" element={<Home />} />
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

describe('Home', () => {
  it('shows a connected user', async () => {
    stubFetch(async () => new Response(me(true), { status: 200 }))
    renderHome()

    expect(await screen.findByRole('heading', { name: 'Signed in as Alice Example' })).toBeInTheDocument()
    expect(screen.getByText(/Your Slack account is linked\./)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Sign in again' })).toBeNull()
  })

  it('asks a user whose Slack account is no longer linked to sign in again', async () => {
    stubFetch(async () => new Response(me(false), { status: 200 }))
    renderHome()

    expect(await screen.findByRole('alert')).toHaveTextContent('Your Slack account is no longer linked.')
    expect(screen.getByRole('button', { name: 'Sign in again' })).toBeInTheDocument()
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
    renderHome()

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
    renderHome()

    fireEvent.click(await screen.findByRole('button', { name: 'Sign out' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Could not sign out. Try again.')
    await waitFor(() => expect(screen.getByRole('button', { name: 'Sign out' })).toBeEnabled())
  })
})
