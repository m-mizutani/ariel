import { describe, expect, it } from 'vitest'
import { listIntegrations } from './integrations'

const me = (connected: boolean) => ({ team_id: 'T1', user_id: 'U1', name: 'Alice', slack_connected: connected })

describe('listIntegrations', () => {
  it('lists the services in display order', () => {
    expect(listIntegrations(me(true)).map((i) => [i.id, i.name])).toEqual([
      ['slack', 'Slack'],
      ['google_workspace', 'Google Workspace'],
      ['notion', 'Notion'],
      ['github', 'GitHub'],
    ])
  })

  it('marks Slack connected when the user has a Slack token', () => {
    expect(listIntegrations(me(true)).map((i) => i.status)).toEqual([
      'connected',
      'coming_soon',
      'coming_soon',
      'coming_soon',
    ])
  })

  it('marks Slack not connected when the user has no Slack token', () => {
    expect(listIntegrations(me(false)).map((i) => i.status)).toEqual([
      'not_connected',
      'coming_soon',
      'coming_soon',
      'coming_soon',
    ])
  })

  it('explains that Slack cannot be disconnected only while it is connected', () => {
    expect(listIntegrations(me(true))[0].description).toContain('you cannot disconnect Slack on this page')
    expect(listIntegrations(me(false))[0].description).not.toContain('cannot disconnect')
  })
})
