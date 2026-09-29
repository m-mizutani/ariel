import type { Me } from './api'

export type IntegrationID = 'slack' | 'google_workspace' | 'notion' | 'github'
export type IntegrationStatus = 'connected' | 'not_connected' | 'coming_soon'

export type Integration = {
  id: IntegrationID
  name: string
  description: string
  status: IntegrationStatus
}

const comingSoonDescription = 'You can connect this service in a later release.'
const slackDescription = 'When you mention @ariel in a Slack channel, Ariel replies in the thread.'
const slackConnectedNote = 'You sign in to Ariel with Slack, so you cannot disconnect Slack on this page.'

// listIntegrations returns every service shown on the settings page, in display
// order. Only Slack has a server-side connection today; its status comes from
// /api/auth/me.
export function listIntegrations(me: Me): Integration[] {
  return [
    {
      id: 'slack',
      name: 'Slack',
      description: me.slack_connected ? `${slackDescription} ${slackConnectedNote}` : slackDescription,
      status: me.slack_connected ? 'connected' : 'not_connected',
    },
    { id: 'google_workspace', name: 'Google Workspace', description: comingSoonDescription, status: 'coming_soon' },
    { id: 'notion', name: 'Notion', description: comingSoonDescription, status: 'coming_soon' },
    { id: 'github', name: 'GitHub', description: comingSoonDescription, status: 'coming_soon' },
  ]
}
