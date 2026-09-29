import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { describe, expect, it, vi } from 'vitest'
import { AuthProvider } from '../contexts/auth-context'
import AuthGuard from './AuthGuard'

function renderGuard() {
  return render(
    <MemoryRouter initialEntries={['/']}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<p>login page</p>} />
          <Route
            path="/"
            element={
              <AuthGuard>
                <p>protected content</p>
              </AuthGuard>
            }
          />
        </Routes>
      </AuthProvider>
    </MemoryRouter>,
  )
}

const meBody = JSON.stringify({ team_id: 'T1', user_id: 'U1', name: 'Alice', slack_connected: true })

describe('AuthGuard', () => {
  it('shows the content to a signed-in user', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(meBody, { status: 200 })))
    renderGuard()
    expect(await screen.findByText('protected content')).toBeInTheDocument()
  })

  it('shows loading while the sign-in status is checked', () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => {})))
    renderGuard()
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  it('sends a signed-out user to the login page', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"error":"unauthenticated"}', { status: 401 })))
    renderGuard()
    expect(await screen.findByText('login page')).toBeInTheDocument()
  })

  it.each([
    ['a server error', async () => new Response('{"error":"internal_error"}', { status: 500 })],
    [
      'a network error',
      async () => {
        throw new TypeError('Failed to fetch')
      },
    ],
  ])('offers a retry after %s', async (_, failure) => {
    const fetchMock = vi.fn(failure)
    vi.stubGlobal('fetch', fetchMock)
    renderGuard()

    expect(await screen.findByRole('alert')).toHaveTextContent('Could not check your sign-in status.')
    fetchMock.mockImplementation(async () => new Response(meBody, { status: 200 }))
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

    expect(await screen.findByText('protected content')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
