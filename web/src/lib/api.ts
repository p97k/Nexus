import type {
  ActionProposal,
  Agent,
  BankStat,
  Message,
  Project,
  Run,
  User,
} from './types'

const BASE = '/api'

function getToken(): string | null {
  if (typeof window === 'undefined') return null
  return localStorage.getItem('nexus_token')
}

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  }
  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await fetch(`${BASE}${path}`, { ...options, headers })
  if (!res.ok) {
    if (res.status === 401) {
      // Token is missing, expired, or invalid — clear local auth and redirect to login
      localStorage.removeItem('nexus_token')
      localStorage.removeItem('nexus_user')
      window.location.href = '/login'
      throw new Error('Session expired. Please log in again.')
    }
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error ?? res.statusText)
  }
  const json = await res.json()
  return json.data as T
}

// ─── Auth ─────────────────────────────────────────────────────────────────────

export async function login(email: string, password: string): Promise<{ token: string; user: User }> {
  const res = await fetch(`${BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: 'Login failed' }))
    throw new Error(body.error)
  }
  const json = await res.json()
  return json.data as { token: string; user: User }
}

export async function getMe(): Promise<User> {
  return request<User>('/auth/me')
}

export async function listProviders(): Promise<string[]> {
  return request<string[]>('/providers')
}

// ─── Projects ─────────────────────────────────────────────────────────────────

export async function listProjects(): Promise<Project[]> {
  return request<Project[]>('/projects')
}

export async function listAgents(projectId: string): Promise<Agent[]> {
  return request<Agent[]>(`/projects/${projectId}/agents`)
}

export async function getAgent(id: string): Promise<Agent> {
  return request<Agent>(`/agents/${id}`)
}

// ─── Runs ─────────────────────────────────────────────────────────────────────

export async function createRun(params: {
  agent_id: string
  title?: string
  provider?: string
  model?: string
}): Promise<Run> {
  return request<Run>('/runs', {
    method: 'POST',
    body: JSON.stringify(params),
  })
}

export async function listRuns(): Promise<Run[]> {
  const data = await request<Run[]>('/runs')
  return data ?? []
}

export async function getRun(id: string): Promise<Run> {
  return request<Run>(`/runs/${id}`)
}

export async function getRunMessages(runId: string): Promise<Message[]> {
  const data = await request<Message[]>(`/runs/${runId}/messages`)
  return data ?? []
}

export async function sendMessage(runId: string, content: string): Promise<void> {
  await request(`/runs/${runId}/messages`, {
    method: 'POST',
    body: JSON.stringify({ content }),
  })
}

// ─── Proposals ────────────────────────────────────────────────────────────────

export async function listProposals(): Promise<ActionProposal[]> {
  const data = await request<ActionProposal[]>('/proposals')
  return data ?? []
}

export async function approveProposal(id: string): Promise<unknown> {
  return request(`/proposals/${id}/approve`, { method: 'POST' })
}

export async function rejectProposal(id: string): Promise<unknown> {
  return request(`/proposals/${id}/reject`, { method: 'POST' })
}

// ─── Dashboard ────────────────────────────────────────────────────────────────

export async function getCashbackDashboard(): Promise<BankStat[]> {
  const data = await request<BankStat[]>('/dashboard/cashback')
  return data ?? []
}

// ─── SSE ──────────────────────────────────────────────────────────────────────

export function createRunStream(
  runId: string,
  onEvent: (event: { type: string; payload: unknown }) => void,
  onDone: () => void,
  onError: (err: Error) => void,
): () => void {
  const token = getToken()
  const url = `${BASE}/runs/${runId}/stream`

  const es = new EventSource(
    url + (token ? `?token=${encodeURIComponent(token)}` : ''),
  )

  es.onmessage = (e) => {
    try {
      const event = JSON.parse(e.data)
      onEvent(event)
      if (event.type === 'done' || event.type === 'error') {
        es.close()
        if (event.type === 'done') onDone()
        else onError(new Error(JSON.stringify(event.payload)))
      }
    } catch {
      // ignore malformed
    }
  }

  es.onerror = () => {
    es.close()
    onError(new Error('Stream connection lost'))
  }

  return () => es.close()
}
