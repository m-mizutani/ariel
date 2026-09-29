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
