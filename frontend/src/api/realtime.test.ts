import { describe, expect, it, vi } from 'vitest'
import { AuthenticatedRealtimeSession, type RealtimeConnection, websocketUrl } from './realtime'

describe('realtime connection', () => {
  it('builds a membership-wide URL without an invented project filter', () => {
    const url = new URL(websocketUrl(undefined, 'token with spaces'))

    expect(url.pathname).toBe('/ws')
    expect(url.searchParams.get('token')).toBe('token with spaces')
    expect(url.searchParams.has('project_id')).toBe(false)
  })

  it('still supports an explicitly scoped project stream', () => {
    const url = new URL(websocketUrl('project-1', 'token'))

    expect(url.searchParams.get('project_id')).toBe('project-1')
  })

  it('owns one connection for the authenticated session and tears it down', async () => {
    const connections: Array<RealtimeConnection & { connect: ReturnType<typeof vi.fn>; disconnect: ReturnType<typeof vi.fn> }> = []
    const session = new AuthenticatedRealtimeSession(() => {
      const connection = {
        connect: vi.fn().mockResolvedValue(undefined),
        disconnect: vi.fn(),
      }
      connections.push(connection)
      return connection
    })

    session.sync(false)
    expect(connections).toHaveLength(0)

    session.sync(true)
    session.sync(true)
    expect(connections).toHaveLength(1)
    expect(connections[0].connect).toHaveBeenCalledOnce()

    session.sync(false)
    expect(connections[0].disconnect).toHaveBeenCalledOnce()

    session.sync(true)
    expect(connections).toHaveLength(2)
    session.disconnect()
    expect(connections[1].disconnect).toHaveBeenCalledOnce()
  })
})
