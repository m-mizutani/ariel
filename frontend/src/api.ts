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

// Every API route of the server is under this prefix.
const apiV1 = '/api/v1'
const authPath = `${apiV1}/auth`

export async function fetchMe(): Promise<MeResult> {
  try {
    const res = await fetch(`${authPath}/me`, { credentials: 'include' })
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
  const res = await fetch(`${authPath}/logout`, { method: 'POST', credentials: 'include' })
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`)
  }
}

export function startLogin(): void {
  window.location.assign(`${authPath}/login`)
}

export type GoogleWorkspaceStatus = {
  // false when the server has no Google OAuth client configured
  available: boolean
  connected: boolean
  // the connected Google account; empty when not connected
  email: string
}

const googleWorkspacePath = `${apiV1}/integrations/google-workspace`

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

export type NotionStatus = {
  // false when the server has no Notion integration configured
  available: boolean
  connected: boolean
  // true when Notion stopped accepting the stored authorization
  needs_reconnect: boolean
  // the Notion user and workspace of the connection; empty when not connected
  user_name: string
  workspace_name: string
}

const notionPath = `${apiV1}/integrations/notion`

// Throws when the request fails or the response is not 2xx.
export async function fetchNotionStatus(): Promise<NotionStatus> {
  const res = await fetch(notionPath, { credentials: 'include' })
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`)
  }
  return (await res.json()) as NotionStatus
}

export async function disconnectNotion(): Promise<void> {
  const res = await fetch(`${notionPath}/disconnect`, { method: 'POST', credentials: 'include' })
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`)
  }
}

export function startNotionConnect(): void {
  window.location.assign(`${notionPath}/connect`)
}

export type GitHubStatus = {
  // false when the server has no GitHub App configured
  available: boolean
  connected: boolean
  // the login of the connected GitHub account; empty when not connected
  login: string
}

const githubPath = `${apiV1}/integrations/github`

// Throws when the request fails or the response is not 2xx.
export async function fetchGitHubStatus(): Promise<GitHubStatus> {
  const res = await fetch(githubPath, { credentials: 'include' })
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`)
  }
  return (await res.json()) as GitHubStatus
}

export async function disconnectGitHub(): Promise<void> {
  const res = await fetch(`${githubPath}/disconnect`, { method: 'POST', credentials: 'include' })
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`)
  }
}

export function startGitHubConnect(): void {
  window.location.assign(`${githubPath}/connect`)
}
