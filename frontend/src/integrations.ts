import type { GoogleWorkspaceStatus, Me } from './api'

export type IntegrationID = 'slack' | 'google_workspace' | 'notion' | 'github'
export type IntegrationStatus =
  | 'connected'
  | 'not_connected'
  | 'coming_soon'
  | 'unavailable'
  | 'checking'
  | 'check_failed'

export type Integration = {
  id: IntegrationID
  name: string
  description: string
  status: IntegrationStatus
  // the connected account, shown under the status
  account?: string
}

// The Google Workspace status comes from its own API, so the settings page
// holds it separately from /api/auth/me.
export type GoogleWorkspaceState =
  | { kind: 'loading' }
  | { kind: 'error' }
  | { kind: 'loaded'; status: GoogleWorkspaceStatus }

const comingSoonDescription = 'You can connect this service in a later release.'
const slackDescription = 'When you mention @ariel in a Slack channel, Ariel replies in the thread.'
const slackConnectedNote = 'You sign in to Ariel with Slack, so you cannot disconnect Slack on this page.'
const googleDescription =
  'Gives Ariel read-only access to your Google Calendar, Gmail, and Drive files, including Docs, Sheets, and Slides. ' +
  'No Ariel feature uses this access yet. Ariel cannot create, change, or send anything.'
const googleUnavailableDescription = 'Your Ariel administrator has not set up this integration.'

function googleWorkspace(google: GoogleWorkspaceState): Integration {
  const base = { id: 'google_workspace', name: 'Google Workspace', description: googleDescription } as const
  switch (google.kind) {
    case 'loading':
      return { ...base, status: 'checking' }
    case 'error':
      return { ...base, status: 'check_failed' }
    case 'loaded':
      if (!google.status.available) {
        return { ...base, description: googleUnavailableDescription, status: 'unavailable' }
      }
      if (google.status.connected) {
        return { ...base, status: 'connected', account: google.status.email }
      }
      return { ...base, status: 'not_connected' }
  }
}

// listIntegrations returns every service shown on the settings page, in display
// order. Slack's status comes from /api/auth/me and Google Workspace's from
// /api/integrations/google-workspace.
export function listIntegrations(me: Me, google: GoogleWorkspaceState): Integration[] {
  return [
    {
      id: 'slack',
      name: 'Slack',
      description: me.slack_connected ? `${slackDescription} ${slackConnectedNote}` : slackDescription,
      status: me.slack_connected ? 'connected' : 'not_connected',
    },
    googleWorkspace(google),
    { id: 'notion', name: 'Notion', description: comingSoonDescription, status: 'coming_soon' },
    { id: 'github', name: 'GitHub', description: comingSoonDescription, status: 'coming_soon' },
  ]
}
