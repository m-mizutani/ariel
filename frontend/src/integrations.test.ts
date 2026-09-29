import { describe, expect, it } from 'vitest'
import { listIntegrations, type GoogleWorkspaceState } from './integrations'

const me = (connected: boolean) => ({ team_id: 'T1', user_id: 'U1', name: 'Alice', slack_connected: connected })

const googleNotConnected: GoogleWorkspaceState = {
  kind: 'loaded',
  status: { available: true, connected: false, email: '' },
}

describe('listIntegrations', () => {
  it('lists the services in display order', () => {
    expect(listIntegrations(me(true), googleNotConnected).map((i) => [i.id, i.name])).toEqual([
      ['slack', 'Slack'],
      ['google_workspace', 'Google Workspace'],
      ['notion', 'Notion'],
      ['github', 'GitHub'],
    ])
  })

  it('marks Slack connected when the user has a Slack token', () => {
    expect(listIntegrations(me(true), googleNotConnected).map((i) => i.status)).toEqual([
      'connected',
      'not_connected',
      'coming_soon',
      'coming_soon',
    ])
  })

  it('marks Slack not connected when the user has no Slack token', () => {
    expect(listIntegrations(me(false), googleNotConnected)[0].status).toEqual('not_connected')
  })

  it('explains that Slack cannot be disconnected only while it is connected', () => {
    expect(listIntegrations(me(true), googleNotConnected)[0].description).toContain(
      'you cannot disconnect Slack on this page',
    )
    expect(listIntegrations(me(false), googleNotConnected)[0].description).not.toContain('cannot disconnect')
  })

  describe('Google Workspace', () => {
    const google = (state: GoogleWorkspaceState) => listIntegrations(me(true), state)[1]

    it.each([
      [{ kind: 'loading' } as const, 'checking'],
      [{ kind: 'error' } as const, 'check_failed'],
      [{ kind: 'loaded', status: { available: false, connected: false, email: '' } } as const, 'unavailable'],
      [googleNotConnected, 'not_connected'],
      [{ kind: 'loaded', status: { available: true, connected: true, email: 'alice@example.com' } } as const, 'connected'],
    ])('maps %j to %s', (state, status) => {
      expect(google(state).status).toEqual(status)
    })

    it('shows the connected account only when connected', () => {
      expect(
        google({ kind: 'loaded', status: { available: true, connected: true, email: 'alice@example.com' } }).account,
      ).toEqual('alice@example.com')
      expect(google(googleNotConnected).account).toBeUndefined()
    })

    it('describes the read-only access, or that the server has not set it up', () => {
      expect(google(googleNotConnected).description).toContain('read-only access')
      expect(google(googleNotConnected).description).toContain('cannot create, change, or send anything')
      expect(google({ kind: 'loaded', status: { available: false, connected: false, email: '' } }).description).toEqual(
        'Your Ariel administrator has not set up this integration.',
      )
    })

    it('keeps the other services unchanged whatever its state', () => {
      for (const state of [{ kind: 'loading' } as const, { kind: 'error' } as const, googleNotConnected]) {
        const [slack, , notion, github] = listIntegrations(me(true), state)
        expect(slack.status).toEqual('connected')
        expect(notion.status).toEqual('coming_soon')
        expect(github.status).toEqual('coming_soon')
      }
    })
  })
})
