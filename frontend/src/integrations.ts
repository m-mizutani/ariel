import type { GitHubStatus, GoogleWorkspaceStatus, Me, NotionStatus } from './api'

export type IntegrationID = 'slack' | 'google_workspace' | 'notion' | 'github'
export type IntegrationStatus =
  | 'connected'
  | 'not_connected'
  | 'coming_soon'
  | 'unavailable'
  | 'checking'
  | 'check_failed'
  | 'needs_reconnect'

export type Integration = {
  id: IntegrationID
  name: string
  description: string
  status: IntegrationStatus
  // the connected account, shown under the status
  account?: string
}

// The status of a service with its own status API. The settings page holds it
// separately from /api/v1/auth/me.
export type ServiceState<T> = { kind: 'loading' } | { kind: 'error' } | { kind: 'loaded'; status: T }

export type GoogleWorkspaceState = ServiceState<GoogleWorkspaceStatus>
export type NotionState = ServiceState<NotionStatus>
export type GitHubState = ServiceState<GitHubStatus>

const unavailableDescription = 'Your Robin administrator has not set up this integration.'
const slackDescription = 'When you mention @robin in a Slack channel, Robin replies in the thread.'
const slackConnectedNote = 'You sign in to Robin with Slack, so you cannot disconnect Slack on this page.'
const googleDescription =
  'Gives Robin read-only access to your Google Calendar, Gmail, and Drive files, including Docs, Sheets, and Slides. ' +
  'No Robin feature uses this access yet. Robin cannot create, change, or send anything.'
const notionDescription =
  'Gives Robin read-only access to the Notion pages and databases you share with it. ' +
  'No Robin feature uses this access yet. Robin cannot create or change anything.'
const notionReconnectDescription =
  'Robin can no longer access your Notion pages. Reconnect Notion to give Robin access again.'
const githubDescription =
  'Gives Robin read-only access to the repositories, issues, pull requests, and other GitHub content your GitHub account can see, ' +
  'in organizations where the Robin GitHub App is installed. ' +
  'No Robin feature uses this access yet. Robin cannot create or change anything.'

function googleWorkspace(google: GoogleWorkspaceState): Integration {
  const base = { id: 'google_workspace', name: 'Google Workspace', description: googleDescription } as const
  switch (google.kind) {
    case 'loading':
      return { ...base, status: 'checking' }
    case 'error':
      return { ...base, status: 'check_failed' }
    case 'loaded':
      if (!google.status.available) {
        return { ...base, description: unavailableDescription, status: 'unavailable' }
      }
      if (google.status.connected) {
        return { ...base, status: 'connected', account: google.status.email }
      }
      return { ...base, status: 'not_connected' }
  }
}

// notionAccount names the Notion user and workspace of a connection, such as
// "Alice Example (Acme)". Either name can be missing.
function notionAccount(status: NotionStatus): string | undefined {
  if (status.user_name && status.workspace_name) {
    return `${status.user_name} (${status.workspace_name})`
  }
  return status.user_name || status.workspace_name || undefined
}

function notion(state: NotionState): Integration {
  const base = { id: 'notion', name: 'Notion', description: notionDescription } as const
  switch (state.kind) {
    case 'loading':
      return { ...base, status: 'checking' }
    case 'error':
      return { ...base, status: 'check_failed' }
    case 'loaded':
      if (!state.status.available) {
        return { ...base, description: unavailableDescription, status: 'unavailable' }
      }
      if (!state.status.connected) {
        return { ...base, status: 'not_connected' }
      }
      if (state.status.needs_reconnect) {
        return {
          ...base,
          description: notionReconnectDescription,
          status: 'needs_reconnect',
          account: notionAccount(state.status),
        }
      }
      return { ...base, status: 'connected', account: notionAccount(state.status) }
  }
}

function github(state: GitHubState): Integration {
  const base = { id: 'github', name: 'GitHub', description: githubDescription } as const
  switch (state.kind) {
    case 'loading':
      return { ...base, status: 'checking' }
    case 'error':
      return { ...base, status: 'check_failed' }
    case 'loaded':
      if (!state.status.available) {
        return { ...base, description: unavailableDescription, status: 'unavailable' }
      }
      if (state.status.connected) {
        return { ...base, status: 'connected', account: `@${state.status.login}` }
      }
      return { ...base, status: 'not_connected' }
  }
}

// listIntegrations returns every service shown on the settings page, in display
// order. Slack's status comes from /api/v1/auth/me; Google Workspace, Notion,
// and GitHub have their own status APIs under /api/v1/integrations.
export function listIntegrations(
  me: Me,
  google: GoogleWorkspaceState,
  notionState: NotionState,
  githubState: GitHubState,
): Integration[] {
  return [
    {
      id: 'slack',
      name: 'Slack',
      description: me.slack_connected ? `${slackDescription} ${slackConnectedNote}` : slackDescription,
      status: me.slack_connected ? 'connected' : 'not_connected',
    },
    googleWorkspace(google),
    notion(notionState),
    github(githubState),
  ]
}
