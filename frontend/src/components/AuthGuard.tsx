import type { ReactNode } from 'react'
import { Navigate } from 'react-router'
import { useAuth } from '../contexts/auth-context'

export default function AuthGuard({ children }: { children: ReactNode }) {
  const { state, reload } = useAuth()

  switch (state.kind) {
    case 'loading':
      return (
        <main className="page">
          <p className="muted">Loading…</p>
        </main>
      )
    case 'unauthenticated':
      return <Navigate to="/login" replace />
    case 'error':
      return (
        <main className="page">
          <section className="card">
            <p className="error" role="alert">
              Could not check your sign-in status. Check your connection and retry.
            </p>
            <button type="button" className="button" onClick={() => void reload()}>
              Retry
            </button>
          </section>
        </main>
      )
    case 'authenticated':
      return <>{children}</>
  }
}
