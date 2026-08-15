import type { RealtimeEvent } from '../domain/types'

type Listener = (event: RealtimeEvent) => void

export function websocketUrl(projectId: string | undefined, token: string): string {
  const configured = import.meta.env.VITE_WS_URL?.replace(/\/$/, '')
  const base = configured || `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`
  const url = new URL(base, window.location.href)
  url.searchParams.set('token', token)
  if (projectId) url.searchParams.set('project_id', projectId)
  return url.toString()
}

export interface RealtimeConnection {
  connect(): Promise<void>
  disconnect(): void
}

/** Keeps exactly one realtime connection alive while the application is authenticated. */
export class AuthenticatedRealtimeSession {
  private connection?: RealtimeConnection

  constructor(private readonly factory: () => RealtimeConnection) {}

  sync(authenticated: boolean) {
    if (!authenticated) {
      this.disconnect()
      return
    }
    if (this.connection) return

    const connection = this.factory()
    this.connection = connection
    void connection.connect().catch(() => {
      if (this.connection === connection) {
        connection.disconnect()
        this.connection = undefined
      }
    })
  }

  disconnect() {
    this.connection?.disconnect()
    this.connection = undefined
  }
}

export class ProjectSocket implements RealtimeConnection {
  private socket?: WebSocket
  private reconnectTimer?: number
  private attempts = 0
  private stopped = false

  constructor(
    private readonly projectId: string | undefined,
    private readonly token: () => Promise<string | undefined>,
    private readonly listener: Listener,
  ) {}

  async connect() {
    this.stopped = false
    const token = await this.token()
    if (!token || this.stopped) return
    this.socket?.close()
    const socket = new WebSocket(websocketUrl(this.projectId, token))
    this.socket = socket
    socket.onopen = () => { this.attempts = 0 }
    socket.onmessage = (message) => {
      try { this.listener(JSON.parse(String(message.data)) as RealtimeEvent) }
      catch { /* Ignore keepalives and non-JSON frames. */ }
    }
    socket.onclose = () => {
      if (this.stopped || this.socket !== socket) return
      const wait = Math.min(1000 * 2 ** this.attempts++, 15000)
      this.reconnectTimer = window.setTimeout(() => { void this.connect() }, wait)
    }
  }

  disconnect() {
    this.stopped = true
    if (this.reconnectTimer !== undefined) window.clearTimeout(this.reconnectTimer)
    this.reconnectTimer = undefined
    this.socket?.close()
    this.socket = undefined
  }
}
