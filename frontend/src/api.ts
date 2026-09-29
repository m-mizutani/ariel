export type Me = {
  team_id: string
  user_id: string
  name: string
  slack_connected: boolean
}

export type MeResult =
  | { kind: 'authenticated'; me: Me }
  | { kind: 'unauthenticated' }
  | { kind: 'error'; message: string }

export async function fetchMe(): Promise<MeResult> {
  try {
    const res = await fetch('/api/auth/me', { credentials: 'include' })
    if (res.status === 401) {
      return { kind: 'unauthenticated' }
    }
    if (!res.ok) {
      return { kind: 'error', message: `HTTP ${res.status}` }
    }
    return { kind: 'authenticated', me: (await res.json()) as Me }
  } catch (e) {
    return { kind: 'error', message: e instanceof Error ? e.message : String(e) }
  }
}

export async function logout(): Promise<void> {
  const res = await fetch('/api/auth/logout', { method: 'POST', credentials: 'include' })
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`)
  }
}

export function startLogin(): void {
  window.location.assign('/api/auth/login')
}

export type GoogleWorkspaceStatus = {
  // false when the server has no Google OAuth client configured
  available: boolean
  connected: boolean
  // the connected Google account; empty when not connected
  email: string
}

const googleWorkspacePath = '/api/integrations/google-workspace'

// Throws when the request fails or the response is not 2xx.
export async function fetchGoogleWorkspaceStatus(): Promise<GoogleWorkspaceStatus> {
  const res = await fetch(googleWorkspacePath, { credentials: 'include' })
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`)
  }
  return (await res.json()) as GoogleWorkspaceStatus
}

export async function disconnectGoogleWorkspace(): Promise<void> {
  const res = await fetch(`${googleWorkspacePath}/disconnect`, { method: 'POST', credentials: 'include' })
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`)
  }
}

export function startGoogleWorkspaceConnect(): void {
  window.location.assign(`${googleWorkspacePath}/connect`)
}
