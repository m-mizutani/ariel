import { useState } from 'react'
import { useNavigate } from 'react-router'
import { logout, startLogin } from '../api'
import { useAuth } from '../contexts/auth-context'

export default function Home() {
  const { state, reload } = useAuth()
  const navigate = useNavigate()
  const [signingOut, setSigningOut] = useState(false)
  const [signOutFailed, setSignOutFailed] = useState(false)

  if (state.kind !== 'authenticated') {
    return null
  }
  const { me } = state

  const onSignOut = async () => {
    setSigningOut(true)
    setSignOutFailed(false)
    try {
      await logout()
    } catch {
      setSignOutFailed(true)
      setSigningOut(false)
      return
    }
    await reload()
    navigate('/login', { replace: true })
  }

  return (
    <main className="page">
      <section className="card">
        <h1 className="title">Signed in as {me.name}</h1>
        {me.slack_connected ? (
          <p className="success">
            Your Slack account is linked. Mention @ariel in a channel, and it replies in the thread.
          </p>
        ) : (
          <>
            <p className="error" role="alert">
              Your Slack account is no longer linked. Sign in again to link it.
            </p>
            <button type="button" className="button" onClick={startLogin}>
              Sign in again
            </button>
          </>
        )}
        {signOutFailed && (
          <p className="error" role="alert">
            Could not sign out. Try again.
          </p>
        )}
        <button type="button" className="button secondary" onClick={() => void onSignOut()} disabled={signingOut}>
          {signingOut ? 'Signing out…' : 'Sign out'}
        </button>
      </section>
    </main>
  )
}
