import { useState } from 'react'
import { useNavigate } from 'react-router'
import { logout, startLogin } from '../api'
import { useAuth } from '../contexts/auth-context'
import { listIntegrations, type Integration, type IntegrationID, type IntegrationStatus } from '../integrations'

// Services whose connection can be started now. Every other service shows a
// disabled button.
const connectActions: Partial<Record<IntegrationID, () => void>> = { slack: () => startLogin() }

const statusLabels: Record<IntegrationStatus, { text: string; className: string }> = {
  connected: { text: 'Connected', className: 'success' },
  not_connected: { text: 'Not connected', className: 'error' },
  coming_soon: { text: 'Coming soon', className: 'muted' },
}

export default function Settings() {
  const { state, reload } = useAuth()
  const navigate = useNavigate()
  const [connecting, setConnecting] = useState<IntegrationID | null>(null)
  const [signingOut, setSigningOut] = useState(false)
  const [signOutFailed, setSignOutFailed] = useState(false)

  if (state.kind !== 'authenticated') {
    return null
  }
  const { me } = state

  const onConnect = (id: IntegrationID) => {
    const action = connectActions[id]
    if (!action) {
      return
    }
    setConnecting(id)
    action()
  }

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

  const renderAction = (integration: Integration) => {
    if (integration.status === 'connected') {
      return null
    }
    const isConnecting = connecting === integration.id
    const canConnect = integration.status === 'not_connected' && connectActions[integration.id] !== undefined
    return (
      <button
        type="button"
        className="button"
        onClick={() => onConnect(integration.id)}
        disabled={!canConnect || isConnecting}
      >
        {isConnecting ? `Redirecting to ${integration.name}…` : `Connect ${integration.name}`}
      </button>
    )
  }

  return (
    <main className="page">
      <section className="card">
        <h1 className="title">Settings</h1>
        <p className="muted">Signed in as {me.name}</p>
        <h2>Integrations</h2>
        <ul className="integration-list">
          {listIntegrations(me).map((integration) => {
            const label = statusLabels[integration.status]
            return (
              <li key={integration.id} className="integration" aria-labelledby={`integration-${integration.id}`}>
                <h3 id={`integration-${integration.id}`}>{integration.name}</h3>
                <p className={label.className}>{label.text}</p>
                <p className="muted">{integration.description}</p>
                {renderAction(integration)}
              </li>
            )
          })}
        </ul>
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
