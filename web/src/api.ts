export const EDGE_KEY = 'inja.edge_key'
export const origin = import.meta.env.VITE_GATEWAY_ORIGIN ?? ''

export class AuthError extends Error {
  constructor() {
    super('unauthorized')
    this.name = 'AuthError'
  }
}

let onAuth: (() => void) | null = null
export function setOnAuth(fn: () => void) {
  onAuth = fn
}

export function authHeader(): Record<string, string> {
  const k = sessionStorage.getItem(EDGE_KEY)
  return k ? { Authorization: `Bearer ${k}` } : {}
}

export type Profile = {
  provider: string
  account_id: string
  source: string
  expiry: string | null
  usable: boolean
  has_refresh: boolean
  has_access: boolean
  access_state: string
  cooldown_until: string | null
  disabled: boolean
  updated_at: string | null
}

export type UsageEvent = {
  request_id: string
  time: string
  dialect_in: string
  provider: string
  model: string
  upstream_model: string
  tokens_in: number
  tokens_out: number
  stream: boolean
  status: string
  http_status: number
  latency_ms: number
}

export type UsageTotals = {
  tokens_in: number
  tokens_out: number
  requests: number
  by_provider: Record<string, number>
  by_model: Record<string, number>
  by_status: Record<string, number>
}

export type OAuthStatus = {
  state: 'pending' | 'complete' | 'error' | 'idle'
  kind?: string
  authorize_url?: string
  user_code?: string
  verification_uri?: string
  error?: string
}

export type Meta = {
  version: string
  dashboard: boolean
  sqlite: boolean
  noweb: boolean
  nodb: boolean
  ring_size: number
  sqlite_path_set: boolean
}

export type Settings = {
  ring_size: number
  sqlite_path: string
  sqlite_enabled: boolean
  jsonl_output: string
}

async function readError(res: Response): Promise<string> {
  try {
    const j = await res.json()
    return j?.error?.message || res.statusText
  } catch {
    return res.statusText
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    ...authHeader(),
    ...(init?.headers as Record<string, string> | undefined),
  }
  if (init?.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json'
  const res = await fetch(`${origin}${path}`, { ...init, headers })
  if (res.status === 401) {
    onAuth?.()
    throw new AuthError()
  }
  if (res.status === 204) return undefined as T
  if (!res.ok) throw new Error(await readError(res))
  if (res.headers.get('content-type')?.includes('json')) return res.json() as Promise<T>
  return undefined as T
}

export async function startStream(
  onEvent: (ev: UsageEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  const res = await fetch(`${origin}/v1/dashboard/logs/stream`, {
    headers: authHeader(),
    signal,
  })
  if (res.status === 401) {
    onAuth?.()
    throw new AuthError()
  }
  if (!res.ok || !res.body) throw new Error('stream failed')
  const reader = res.body.getReader()
  const dec = new TextDecoder()
  let buf = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buf += dec.decode(value, { stream: true })
    const lines = buf.split('\n')
    buf = lines.pop() ?? ''
    for (const line of lines) {
      if (!line.startsWith('data: ')) continue
      try {
        onEvent(JSON.parse(line.slice(6)))
      } catch {
        /* heartbeat / malformed */
      }
    }
  }
}

export const qs = (p: Record<string, string>) => {
  const u = new URLSearchParams()
  for (const [k, v] of Object.entries(p)) if (v) u.set(k, v)
  const s = u.toString()
  return s ? `?${s}` : ''
}
