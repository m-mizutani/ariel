import { useState } from 'react'
import { Navigate, useSearchParams } from 'react-router'
import { startLogin } from '../api'
import { useAuth } from '../contexts/auth-context'

function errorMessage(code: string | null): string | null {
  if (code === null) {
    return null
  }
  if (code === 'access_denied') {
    return 'Sign-in was cancelled. Sign in again to connect your Slack account.'
  }
  return 'Sign-in failed. Try again. If it keeps failing, ask the administrator to check the server logs.'
}

export default function Login() {
  const { state } = useAuth()
  const [params] = useSearchParams()
  const [redirecting, setRedirecting] = useState(false)

  if (state.kind === 'authenticated') {
    return <Navigate to="/" replace />
  }

  const error = errorMessage(params.get('error'))

  const onClick = () => {
    setRedirecting(true)
    startLogin()
  }

  return (
    <main className="page">
      <section className="card">
        <h1 className="title">Ariel</h1>
        <p>Sign in with your Slack account to use Ariel. Ariel replies when you mention it in Slack.</p>
        <p className="muted">
          Slack asks you to allow Ariel to search messages as you. Ariel uses this permission only for requests you
          make.
        </p>
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        <button type="button" className="button" onClick={onClick} disabled={redirecting}>
          {redirecting ? 'Redirecting to Slack…' : 'Sign in with Slack'}
        </button>
      </section>
    </main>
  )
}
