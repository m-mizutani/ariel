import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AuthProvider } from '../contexts/auth-context'
import Login from './Login'

const startLogin = vi.fn()
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  startLogin: () => startLogin(),
}))

function renderLogin(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/settings" element={<p>settings page</p>} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>,
  )
}

describe('Login', () => {
  beforeEach(() => {
    startLogin.mockReset()
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"error":"unauthenticated"}', { status: 401 })))
  })

  it('shows the sign-in button', async () => {
    renderLogin('/login')
    expect(await screen.findByRole('button', { name: 'Sign in with Slack' })).toBeEnabled()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('explains a cancelled sign-in', async () => {
    renderLogin('/login?error=access_denied')
    expect(await screen.findByRole('alert')).toHaveTextContent('Sign-in was cancelled.')
  })

  it.each(['login_failed', 'something_else'])('explains a failed sign-in for %s', async (code) => {
    renderLogin(`/login?error=${code}`)
    expect(await screen.findByRole('alert')).toHaveTextContent('Sign-in failed. Try again.')
  })

  it('starts the Slack authorization and disables the button', async () => {
    renderLogin('/login')
    const button = await screen.findByRole('button', { name: 'Sign in with Slack' })
    fireEvent.click(button)

    expect(startLogin).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('button', { name: 'Redirecting to Slack…' })).toBeDisabled()
  })

  it('sends a signed-in user to the settings page', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(JSON.stringify({ team_id: 'T1', user_id: 'U1', name: 'Alice', slack_connected: true }), {
            status: 200,
          }),
      ),
    )
    renderLogin('/login')
    expect(await screen.findByText('settings page')).toBeInTheDocument()
  })
})
