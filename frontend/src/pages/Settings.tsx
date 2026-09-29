import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import {
  disconnectGoogleWorkspace,
  fetchGoogleWorkspaceStatus,
  logout,
  startGoogleWorkspaceConnect,
  startLogin,
} from '../api'
import { useAuth } from '../contexts/auth-context'
import {
  listIntegrations,
  type GoogleWorkspaceState,
  type Integration,
  type IntegrationID,
  type IntegrationStatus,
} from '../integrations'

// Services whose connection can be started now. Every other service shows a
// disabled button.
const connectActions: Partial<Record<IntegrationID, () => void>> = {
  slack: () => startLogin(),
  google_workspace: () => startGoogleWorkspaceConnect(),
}

// Services that can be disconnected on this page. Slack is the sign-in method,
// so it is not here.
const disconnectActions: Partial<Record<IntegrationID, () => Promise<void>>> = {
  google_workspace: disconnectGoogleWorkspace,
}

const statusLabels: Record<IntegrationStatus, { text: string; className: string }> = {
  connected: { text: 'Connected', className: 'success' },
  not_connected: { text: 'Not connected', className: 'error' },
  coming_soon: { text: 'Coming soon', className: 'muted' },
  unavailable: { text: 'Not available', className: 'muted' },
  checking: { text: 'Checking…', className: 'muted' },
  check_failed: { text: 'Could not load the connection status.', className: 'error' },
}

// The server returns to /settings?google_workspace=<result> after a Google
// Workspace connection attempt. Any other value is ignored.
const googleNotices = {
  connected: { text: 'Google Workspace is connected.', className: 'success', role: 'status' },
  access_denied: {
    text: 'Google Workspace was not connected because you cancelled the request on Google.',
    className: 'error',
    role: 'alert',
  },
  missing_scope: {
    text: 'Google Workspace was not connected because you did not allow every requested permission. Connect again and allow all of them.',
    className: 'error',
    role: 'alert',
  },
  account_in_use: {
    text: 'Google Workspace was not connected because this Google account is already connected to another Ariel user. Connect a different Google account.',
    className: 'error',
    role: 'alert',
  },
  failed: { text: 'Could not connect Google Workspace. Try again.', className: 'error', role: 'alert' },
} as const

type GoogleNotice = (typeof googleNotices)[keyof typeof googleNotices]

const googleResultParam = 'google_workspace'

function noticeFor(result: string | null): GoogleNotice | null {
  if (result === null || !Object.prototype.hasOwnProperty.call(googleNotices, result)) {
    return null
  }
  return googleNotices[result as keyof typeof googleNotices]
}

export default function Settings() {
  const { state, reload } = useAuth()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const [connecting, setConnecting] = useState<IntegrationID | null>(null)
  const [disconnecting, setDisconnecting] = useState<IntegrationID | null>(null)
  const [disconnectFailed, setDisconnectFailed] = useState<IntegrationID | null>(null)
  const [signingOut, setSigningOut] = useState(false)
  const [signOutFailed, setSignOutFailed] = useState(false)
  const [google, setGoogle] = useState<GoogleWorkspaceState>({ kind: 'loading' })
  const [notice] = useState(() => noticeFor(searchParams.get(googleResultParam)))

  const loadGoogle = useCallback(async () => {
    setGoogle({ kind: 'loading' })
    try {
      setGoogle({ kind: 'loaded', status: await fetchGoogleWorkspaceStatus() })
    } catch {
      setGoogle({ kind: 'error' })
    }
  }, [])

  useEffect(() => {
    void loadGoogle()
  }, [loadGoogle])

  // Remove the result from the URL once it is shown, so a reload does not
  // show it again.
  useEffect(() => {
    if (searchParams.has(googleResultParam)) {
      setSearchParams({}, { replace: true })
    }
  }, [searchParams, setSearchParams])

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

  const onDisconnect = async (id: IntegrationID) => {
    const action = disconnectActions[id]
    if (!action) {
      return
    }
    setDisconnecting(id)
    setDisconnectFailed(null)
    try {
      await action()
    } catch {
      setDisconnectFailed(id)
      setDisconnecting(null)
      return
    }
    setDisconnecting(null)
    await loadGoogle()
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
    switch (integration.status) {
      case 'connected': {
        if (!disconnectActions[integration.id]) {
          return null
        }
        const isDisconnecting = disconnecting === integration.id
        return (
          <button
            type="button"
            className="button secondary"
            onClick={() => void onDisconnect(integration.id)}
            disabled={isDisconnecting}
          >
            {isDisconnecting ? 'Disconnecting…' : `Disconnect ${integration.name}`}
          </button>
        )
      }
      case 'check_failed':
        return (
          <button type="button" className="button secondary" onClick={() => void loadGoogle()}>
            Check {integration.name} again
          </button>
        )
      case 'checking':
      case 'unavailable':
        return null
      case 'not_connected':
      case 'coming_soon': {
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
    }
  }

  return (
    <main className="page">
      <section className="card">
        <h1 className="title">Settings</h1>
        <p className="muted">Signed in as {me.name}</p>
        <h2>Integrations</h2>
        {notice && (
          <p className={notice.className} role={notice.role}>
            {notice.text}
          </p>
        )}
        <ul className="integration-list">
          {listIntegrations(me, google).map((integration) => {
            const label = statusLabels[integration.status]
            return (
              <li key={integration.id} className="integration" aria-labelledby={`integration-${integration.id}`}>
                <h3 id={`integration-${integration.id}`}>{integration.name}</h3>
                <p className={label.className}>{label.text}</p>
                {integration.account && <p className="muted">Account: {integration.account}</p>}
                <p className="muted">{integration.description}</p>
                {disconnectFailed === integration.id && (
                  <p className="error" role="alert">
                    Could not disconnect {integration.name}. Try again.
                  </p>
                )}
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
