import { describe, expect, it } from 'vitest'
import type { NotionStatus } from './api'
import { listIntegrations, type GitHubState, type GoogleWorkspaceState, type NotionState } from './integrations'

const me = (connected: boolean) => ({ team_id: 'T1', user_id: 'U1', name: 'Alice', slack_connected: connected })

const googleNotConnected: GoogleWorkspaceState = {
  kind: 'loaded',
  status: { available: true, connected: false, email: '' },
}

const notionStatus = (status: Partial<NotionStatus>): NotionState => ({
  kind: 'loaded',
  status: { available: true, connected: false, needs_reconnect: false, user_name: '', workspace_name: '', ...status },
})

const notionNotConnected = notionStatus({})
const notionConnected = notionStatus({ connected: true, user_name: 'Alice Example', workspace_name: 'Acme' })

const githubNotConnected: GitHubState = {
  kind: 'loaded',
  status: { available: true, connected: false, login: '' },
}

const githubConnected: GitHubState = {
  kind: 'loaded',
  status: { available: true, connected: true, login: 'octocat' },
}

describe('listIntegrations', () => {
  it('lists the services in display order', () => {
    expect(listIntegrations(me(true), googleNotConnected, notionNotConnected, githubNotConnected).map((i) => [i.id, i.name])).toEqual([
      ['slack', 'Slack'],
      ['google_workspace', 'Google Workspace'],
      ['notion', 'Notion'],
      ['github', 'GitHub'],
    ])
  })

  it('marks Slack connected when the user has a Slack token', () => {
    expect(listIntegrations(me(true), googleNotConnected, notionNotConnected, githubNotConnected).map((i) => i.status)).toEqual([
      'connected',
      'not_connected',
      'not_connected',
      'not_connected',
    ])
  })

  it('marks Slack not connected when the user has no Slack token', () => {
    expect(listIntegrations(me(false), googleNotConnected, notionNotConnected, githubNotConnected)[0].status).toEqual('not_connected')
  })

  it('explains that Slack cannot be disconnected only while it is connected', () => {
    expect(listIntegrations(me(true), googleNotConnected, notionNotConnected, githubNotConnected)[0].description).toContain(
      'you cannot disconnect Slack on this page',
    )
    expect(listIntegrations(me(false), googleNotConnected, notionNotConnected, githubNotConnected)[0].description).not.toContain(
      'cannot disconnect',
    )
  })

  describe('Google Workspace', () => {
    const google = (state: GoogleWorkspaceState) => listIntegrations(me(true), state, notionNotConnected, githubNotConnected)[1]

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
        const [slack, , notion, github] = listIntegrations(me(true), state, notionConnected, githubNotConnected)
        expect(slack.status).toEqual('connected')
        expect(notion.status).toEqual('connected')
        expect(github.status).toEqual('not_connected')
      }
    })
  })

  describe('Notion', () => {
    const notion = (state: NotionState) => listIntegrations(me(true), googleNotConnected, state, githubNotConnected)[2]

    it.each([
      [{ kind: 'loading' } as const, 'checking'],
      [{ kind: 'error' } as const, 'check_failed'],
      [notionStatus({ available: false }), 'unavailable'],
      [notionNotConnected, 'not_connected'],
      [notionConnected, 'connected'],
      [notionStatus({ connected: true, needs_reconnect: true, user_name: 'Alice Example', workspace_name: 'Acme' }), 'needs_reconnect'],
    ])('maps %j to %s', (state, status) => {
      expect(notion(state).status).toEqual(status)
    })

    it.each([
      ['Alice Example', 'Acme', 'Alice Example (Acme)'],
      ['', 'Acme', 'Acme'],
      ['Alice Example', '', 'Alice Example'],
      ['', '', undefined],
    ])('names the account of user %j in workspace %j as %j', (user, workspace, account) => {
      expect(notion(notionStatus({ connected: true, user_name: user, workspace_name: workspace })).account).toEqual(
        account,
      )
    })

    it('shows the account while a reconnection is needed, but not when not connected', () => {
      expect(
        notion(notionStatus({ connected: true, needs_reconnect: true, user_name: 'Alice Example', workspace_name: 'Acme' }))
          .account,
      ).toEqual('Alice Example (Acme)')
      expect(notion(notionNotConnected).account).toBeUndefined()
    })

    it('describes the read-only access, the reconnection, or that the server has not set it up', () => {
      expect(notion(notionNotConnected).description).toEqual(
        'Gives Ariel read-only access to the Notion pages and databases you share with it. ' +
          'No Ariel feature uses this access yet. Ariel cannot create or change anything.',
      )
      expect(notion(notionConnected).description).toEqual(notion(notionNotConnected).description)
      expect(notion(notionStatus({ connected: true, needs_reconnect: true })).description).toEqual(
        'Ariel can no longer access your Notion pages. Reconnect Notion to give Ariel access again.',
      )
      expect(notion(notionStatus({ available: false })).description).toEqual(
        'Your Ariel administrator has not set up this integration.',
      )
    })

    it('keeps the other services unchanged whatever its state', () => {
      for (const state of [{ kind: 'loading' } as const, { kind: 'error' } as const, notionConnected]) {
        const [slack, google, , github] = listIntegrations(me(true), googleNotConnected, state, githubNotConnected)
        expect(slack.status).toEqual('connected')
        expect(google.status).toEqual('not_connected')
        expect(github.status).toEqual('not_connected')
      }
    })
  })

  describe('GitHub', () => {
    const github = (state: GitHubState) =>
      listIntegrations(me(true), googleNotConnected, notionNotConnected, state)[3]

    it.each([
      [{ kind: 'loading' } as const, 'checking'],
      [{ kind: 'error' } as const, 'check_failed'],
      [{ kind: 'loaded', status: { available: false, connected: false, login: '' } } as const, 'unavailable'],
      [githubNotConnected, 'not_connected'],
      [githubConnected, 'connected'],
    ])('maps %j to %s', (state, status) => {
      expect(github(state).status).toEqual(status)
    })

    it('shows the connected account as @login only when connected', () => {
      expect(github(githubConnected).account).toEqual('@octocat')
      expect(github(githubNotConnected).account).toBeUndefined()
    })

    it('describes the read-only access, or that the server has not set it up', () => {
      expect(github(githubNotConnected).description).toEqual(
        'Gives Ariel read-only access to the repositories, issues, pull requests, and other GitHub content your GitHub account can see, ' +
          'in organizations where the Ariel GitHub App is installed. ' +
          'No Ariel feature uses this access yet. Ariel cannot create or change anything.',
      )
      expect(github(githubConnected).description).toEqual(github(githubNotConnected).description)
      expect(github({ kind: 'loaded', status: { available: false, connected: false, login: '' } }).description).toEqual(
        'Your Ariel administrator has not set up this integration.',
      )
    })

    it('keeps the other services unchanged whatever its state', () => {
      for (const state of [{ kind: 'loading' } as const, { kind: 'error' } as const, githubConnected]) {
        const [slack, google, notion] = listIntegrations(me(true), googleNotConnected, notionNotConnected, state)
        expect(slack.status).toEqual('connected')
        expect(google.status).toEqual('not_connected')
        expect(notion.status).toEqual('not_connected')
      }
    })
  })
})
