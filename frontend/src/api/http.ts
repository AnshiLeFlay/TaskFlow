import { validToken } from './keycloak'

const API_URL = (import.meta.env.VITE_API_URL || '/api/v1').replace(/\/$/, '')

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly code?: string,
    public readonly details?: unknown,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

function apiMessage(body: unknown, fallback: string): string {
  if (!body || typeof body !== 'object') return fallback
  const value = body as Record<string, unknown>
  const nested = value.error
  if (typeof nested === 'string') return nested
  if (nested && typeof nested === 'object' && typeof (nested as Record<string, unknown>).message === 'string') {
    return (nested as Record<string, unknown>).message as string
  }
  for (const key of ['message', 'detail', 'reason']) {
    if (typeof value[key] === 'string') return value[key] as string
  }
  return fallback
}

function apiCode(body: unknown): string | undefined {
  if (!body || typeof body !== 'object') return undefined
  const value = body as Record<string, unknown>
  if (typeof value.code === 'string') return value.code
  if (value.error && typeof value.error === 'object') {
    const code = (value.error as Record<string, unknown>).code
    if (typeof code === 'string') return code
  }
  return undefined
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = await validToken()
  const headers = new Headers(init.headers)
  if (token) headers.set('Authorization', `Bearer ${token}`)
  if (init.body && !(init.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  headers.set('Accept', 'application/json')

  let response: Response
  try {
    response = await fetch(`${API_URL}${path}`, { ...init, headers })
  } catch (error) {
    throw new ApiError(error instanceof Error ? error.message : 'Backend is unavailable', 0, 'NETWORK_ERROR')
  }

  const text = await response.text()
  let body: unknown
  try { body = text ? JSON.parse(text) : undefined } catch { body = text }
  if (!response.ok) {
    throw new ApiError(apiMessage(body, `Request failed (${response.status})`), response.status, apiCode(body), body)
  }
  return body as T
}

export function unwrapList<T>(body: T[] | Record<string, unknown>, key: string): T[] {
  if (Array.isArray(body)) return body
  const candidate = body[key] ?? body.data
  return Array.isArray(candidate) ? candidate as T[] : []
}

export function unwrapItem<T>(body: T | Record<string, unknown>, key: string): T {
  if (body && typeof body === 'object' && key in body) return (body as Record<string, unknown>)[key] as T
  if (body && typeof body === 'object' && 'data' in body) return (body as Record<string, unknown>).data as T
  return body as T
}
