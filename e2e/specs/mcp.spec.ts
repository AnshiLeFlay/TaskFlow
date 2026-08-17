import { createHash, randomBytes, randomUUID } from 'node:crypto'
import { createServer } from 'node:http'
import { execFileSync } from 'node:child_process'
import { expect, test, type APIRequestContext } from '@playwright/test'

const issuer = process.env.MCP_E2E_ISSUER || 'http://localhost:8082/realms/taskflow'
const mcpURL = process.env.MCP_E2E_URL || 'http://localhost:8080/mcp'
const clientID = process.env.MCP_E2E_CLIENT_ID || 'taskflow-mcp-e2e'
const redirectURI = process.env.MCP_E2E_REDIRECT_URI || 'http://127.0.0.1:1457/callback'

type JSONRPCResponse = {
  error?: { code: number; message: string }
  result?: {
    isError?: boolean
    structuredContent?: Record<string, unknown>
    tools?: Array<{ name: string }>
    protocolVersion?: string
  }
}

test('OAuth Code + PKCE controls a complete MCP task workflow', async ({ page, request }) => {
  const verifier = randomBytes(64).toString('base64url')
  const challenge = createHash('sha256').update(verifier).digest('base64url')
  const state = randomBytes(24).toString('base64url')
  const authorizationURL = new URL(`${issuer}/protocol/openid-connect/auth`)
  authorizationURL.search = new URLSearchParams({
    client_id: clientID,
    redirect_uri: redirectURI,
    response_type: 'code',
    scope: 'openid taskflow:mcp',
    state,
    code_challenge: challenge,
    code_challenge_method: 'S256',
  }).toString()

  const redirect = new URL(redirectURI)
  let resolveCallback!: (url: URL) => void
  const callbackReceived = new Promise<URL>((resolve) => { resolveCallback = resolve })
  const callbackServer = createServer((incoming, outgoing) => {
    resolveCallback(new URL(incoming.url || '/', redirectURI))
    outgoing.writeHead(200, { 'Content-Type': 'text/plain' })
    outgoing.end('OAuth callback received')
  })
  await new Promise<void>((resolve, reject) => {
    callbackServer.once('error', reject)
    callbackServer.listen(Number(redirect.port), redirect.hostname, resolve)
  })
  await page.goto(authorizationURL.toString())
  await page.locator('#username').fill('alice')
  await page.locator('#password').fill('alice')
  const [callback] = await Promise.all([callbackReceived, page.locator('#kc-login').click()])
  await new Promise<void>((resolve, reject) => callbackServer.close((error) => error ? reject(error) : resolve()))

  expect(callback.searchParams.get('state')).toBe(state)
  const code = callback.searchParams.get('code')
  expect(code).toBeTruthy()

  const tokenResponse = await request.post(`${issuer}/protocol/openid-connect/token`, {
    form: {
      grant_type: 'authorization_code',
      client_id: clientID,
      redirect_uri: redirectURI,
      code: code!,
      code_verifier: verifier,
    },
  })
  expect(tokenResponse).toBeOK()
  const { access_token: accessToken } = await tokenResponse.json() as { access_token: string }
  const claims = JSON.parse(Buffer.from(accessToken.split('.')[1], 'base64url').toString('utf8')) as {
    aud: string | string[]
    iss: string
    scope: string
    sub: string
  }
  expect(claims.iss).toBe(issuer)
  expect(Array.isArray(claims.aud) ? claims.aud : [claims.aud]).toContain(mcpURL)
  expect(claims.scope.split(' ')).toContain('taskflow:mcp')
  expect(claims.sub).toBe('11111111-1111-4111-8111-111111111111')

  const clientContainer = process.env.MCP_E2E_CLIENT_CONTAINER
  if (clientContainer) {
    const probe = {
      url: mcpURL,
      token: accessToken,
      payload: {
        jsonrpc: '2.0',
        id: 1,
        method: 'initialize',
        params: {
          protocolVersion: '2025-11-25',
          capabilities: {},
          clientInfo: { name: 'taskflow-container-smoke', version: '1.0.0' },
        },
      },
    }
    const python = [
      'import json,sys,urllib.request',
      'probe=json.load(sys.stdin)',
      'body=json.dumps(probe["payload"]).encode()',
      'headers={"Authorization":"Bearer "+probe["token"],"Accept":"application/json, text/event-stream","Content-Type":"application/json","MCP-Protocol-Version":"2025-11-25"}',
      'request=urllib.request.Request(probe["url"],data=body,headers=headers,method="POST")',
      'print(urllib.request.urlopen(request,timeout=10).read().decode())',
    ].join(';')
    const raw = execFileSync('docker', ['exec', '-i', clientContainer, 'python', '-c', python], {
      input: JSON.stringify(probe),
      encoding: 'utf8',
    })
    const containerResponse = JSON.parse(raw) as JSONRPCResponse
    expect(containerResponse.error).toBeUndefined()
    expect(containerResponse.result?.protocolVersion).toBe('2025-11-25')
  }

  let requestID = 0
  const rpc = async (method: string, params: Record<string, unknown>) => {
    const response = await request.post(mcpURL, {
      headers: {
        Authorization: `Bearer ${accessToken}`,
        Accept: 'application/json, text/event-stream',
        'Content-Type': 'application/json',
        'MCP-Protocol-Version': '2025-11-25',
      },
      data: { jsonrpc: '2.0', id: ++requestID, method, params },
    })
    expect(response).toBeOK()
    const message = await response.json() as JSONRPCResponse
    expect(message.error).toBeUndefined()
    return message.result!
  }
  const callTool = async (name: string, args: Record<string, unknown>) => {
    const result = await rpc('tools/call', { name, arguments: args })
    expect(result.isError).not.toBe(true)
    return result.structuredContent!
  }

  const initialized = await rpc('initialize', {
    protocolVersion: '2025-11-25',
    capabilities: {},
    clientInfo: { name: 'taskflow-playwright', version: '1.0.0' },
  })
  expect(initialized.protocolVersion).toBe('2025-11-25')
  const listed = await rpc('tools/list', {})
  expect(listed.tools).toHaveLength(20)

  const suffix = randomUUID().slice(0, 8)
  const project = await callTool('create_project', { name: `MCP E2E ${suffix}`, description: 'OAuth PKCE acceptance' })
  const projectID = project.id as string
  await callTool('add_or_update_project_member', {
    project_id: projectID,
    user_id: '22222222-2222-4222-8222-222222222222',
    role: 'member',
  })
  const members = await callTool('list_project_members', { project_id: projectID })
  expect(members.members).toEqual(expect.arrayContaining([
    expect.objectContaining({ user_id: '22222222-2222-4222-8222-222222222222', role: 'member' }),
  ]))

  const board = await callTool('create_board', {
    project_id: projectID,
    name: `MCP Board ${suffix}`,
    statuses: [{ name: 'To Do', position: 0 }, { name: 'Done', position: 1 }],
  })
  const boardID = board.id as string
  const statuses = board.statuses as Array<{ id: string; name: string }>
  const todoID = statuses.find((status) => status.name === 'To Do')!.id
  const doneID = statuses.find((status) => status.name === 'Done')!.id
  await callTool('create_workflow_rule', {
    board_id: boardID,
    from_status_id: todoID,
    to_status_id: doneID,
    conditions: { requires_comment: true, allowed_roles: ['admin'] },
  })

  const task = await callTool('create_task', {
    board_id: boardID,
    title: `MCP Task ${suffix}`,
    status_id: todoID,
    assignee_id: '22222222-2222-4222-8222-222222222222',
  })
  const taskID = task.id as string
  await callTool('update_task', { task_id: taskID, description: 'Updated over MCP' })
  await callTool('add_task_comment', { task_id: taskID, body: 'Ready to transition' })
  const transitioned = await callTool('transition_task', { task_id: taskID, target_status_id: doneID })
  expect(transitioned).toMatchObject({ id: taskID, status_id: doneID })

  const aggregate = await callTool('get_board', { board_id: boardID })
  expect(aggregate.tasks).toEqual(expect.arrayContaining([
    expect.objectContaining({ id: taskID, status_id: doneID, description: 'Updated over MCP' }),
  ]))
})
