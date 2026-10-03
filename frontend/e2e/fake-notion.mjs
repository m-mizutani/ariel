// A stand-in for Notion's OAuth endpoints, used by the E2E tests. The robin
// server under test is started with --notion-api-url pointing here, so the
// browser is sent to this server for authorization and robin exchanges and
// revokes tokens here.
//
// Endpoints:
//   GET  /v1/oauth/authorize  redirects back to redirect_uri with the next result
//   POST /v1/oauth/token      issues tokens for the codes "ok" and "wrong"
//   POST /v1/oauth/revoke     accepts any token
//   POST /__control           {"next": "ok" | "access_denied" | "wrong_workspace"}
//   GET  /__control           the recorded authorize and revoke requests
//   DELETE /__control         clears the records and the next result
import { createServer } from 'node:http'

const port = Number(process.env.FAKE_NOTION_PORT ?? 18082)
const clientID = process.env.FAKE_NOTION_CLIENT_ID ?? ''
const clientSecret = process.env.FAKE_NOTION_CLIENT_SECRET ?? ''
const workspaceID = process.env.FAKE_NOTION_WORKSPACE_ID ?? ''
const otherWorkspaceID = '99999999-8888-4777-8666-555555555555'
const notionUserID = '7b3c1e2d-4f5a-4b6c-9d8e-1f2a3b4c5d6e'

let next = 'ok'
let authorizes = []
let revokes = []

function json(res, status, body) {
  res.writeHead(status, { 'Content-Type': 'application/json' })
  res.end(JSON.stringify(body))
}

async function readJSON(req) {
  let raw = ''
  for await (const chunk of req) {
    raw += chunk
  }
  return raw ? JSON.parse(raw) : {}
}

function basicAuthOK(req) {
  const expected = 'Basic ' + Buffer.from(`${clientID}:${clientSecret}`).toString('base64')
  return req.headers.authorization === expected
}

function tokenResponse(code) {
  const own = code === 'ok'
  return {
    access_token: `access-${Date.now()}`,
    token_type: 'bearer',
    refresh_token: `refresh-${Date.now()}`,
    bot_id: 'b1c2d3e4-0000-4000-8000-000000000001',
    workspace_id: own ? workspaceID : otherWorkspaceID,
    workspace_name: own ? 'E2E Workspace' : 'Other Workspace',
    workspace_icon: null,
    owner: { type: 'user', user: { object: 'user', id: notionUserID, name: 'E2E User' } },
    duplicated_template_id: null,
    request_id: 'fake',
  }
}

const server = createServer(async (req, res) => {
  const url = new URL(req.url, `http://127.0.0.1:${port}`)
  try {
    if (req.method === 'GET' && url.pathname === '/v1/oauth/authorize') {
      const query = Object.fromEntries(url.searchParams)
      authorizes.push(query)
      const back = new URL(query.redirect_uri)
      back.searchParams.set('state', query.state ?? '')
      if (next === 'access_denied') {
        back.searchParams.set('error', 'access_denied')
      } else {
        back.searchParams.set('code', next === 'wrong_workspace' ? 'wrong' : 'ok')
      }
      next = 'ok'
      res.writeHead(302, { Location: back.toString() })
      res.end()
      return
    }

    if (req.method === 'POST' && url.pathname === '/v1/oauth/token') {
      const body = await readJSON(req)
      if (!basicAuthOK(req)) {
        json(res, 401, { object: 'error', status: 401, code: 'invalid_client', message: 'bad client' })
        return
      }
      if (body.grant_type !== 'authorization_code' || !['ok', 'wrong'].includes(body.code)) {
        json(res, 400, { object: 'error', status: 400, code: 'invalid_grant', message: 'bad code' })
        return
      }
      json(res, 200, tokenResponse(body.code))
      return
    }

    if (req.method === 'POST' && url.pathname === '/v1/oauth/revoke') {
      const body = await readJSON(req)
      if (!basicAuthOK(req)) {
        json(res, 401, { object: 'error', status: 401, code: 'invalid_client', message: 'bad client' })
        return
      }
      revokes.push(body.token)
      json(res, 200, { request_id: 'fake' })
      return
    }

    if (url.pathname === '/__control') {
      if (req.method === 'POST') {
        next = (await readJSON(req)).next ?? 'ok'
        json(res, 200, { next })
        return
      }
      if (req.method === 'DELETE') {
        next = 'ok'
        authorizes = []
        revokes = []
        json(res, 200, {})
        return
      }
      json(res, 200, { authorizes, revokes })
      return
    }

    json(res, 404, { object: 'error', status: 404, code: 'object_not_found', message: 'unknown path' })
  } catch (e) {
    json(res, 500, { object: 'error', status: 500, code: 'internal_server_error', message: String(e) })
  }
})

server.listen(port, '127.0.0.1')
