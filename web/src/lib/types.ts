export type Role = 'admin' | 'ops' | 'pm'

export interface User {
  id: string
  email: string
  name: string
  role: Role
  created_at: string
}

export interface Project {
  id: string
  name: string
  slug: string
  description: string
  adapter_id: string
  created_at: string
}

export interface Agent {
  id: string
  project_id: string
  name: string
  description: string
  system_prompt: string
  default_mode: 'auto' | 'manual'
  default_provider: string
  default_model: string
  allowed_tools: string[]
  max_steps: number
  created_at: string
}

export type RunStatus = 'pending' | 'running' | 'completed' | 'failed'

export interface Run {
  id: string
  project_id: string
  agent_id: string
  user_id: string
  title: string
  status: RunStatus
  mode: string
  provider: string
  model: string
  step_count: number
  tokens_in: number
  tokens_out: number
  created_at: string
  updated_at: string
}

export type MessageRole = 'user' | 'assistant' | 'tool'

export interface ToolCall {
  id: string
  name: string
  input: unknown
}

export interface Message {
  id: string
  run_id: string
  role: MessageRole
  content: string
  tool_calls?: ToolCall[]
  tool_call_id?: string
  tool_name?: string
  created_at: string
}

export type ProposalStatus = 'pending' | 'approved' | 'rejected' | 'executed' | 'failed'

export interface ActionProposal {
  id: string
  run_id: string
  project_id: string
  tool_name: string
  description: string
  params: unknown
  status: ProposalStatus
  acted_by?: string
  acted_at?: string
  result?: unknown
  created_at: string
}

export interface BankStat {
  bank_name: string
  pending_count: number
  failed_count: number
  stuck_count: number
  oldest_pending_minutes?: number
}

export interface StreamEvent {
  type: 'message' | 'tool_call' | 'done' | 'error'
  payload: unknown
}

// Google's backend persists tool_calls as {"_ts": "...", "calls": [...]},
// other providers store a plain array. Normalize to a plain ToolCall[] so the
// UI never crashes on `.length`/`.map` of an object.
export function normalizeToolCalls(raw: unknown): ToolCall[] {
  if (Array.isArray(raw)) return raw as ToolCall[]
  if (raw && typeof raw === 'object') {
    const obj = raw as Record<string, unknown>
    if (Array.isArray(obj.calls)) return obj.calls as ToolCall[]
  }
  return []
}
