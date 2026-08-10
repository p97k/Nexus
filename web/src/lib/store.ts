import { create } from 'zustand'
import type { User, Run, Message, ActionProposal } from './types'
import { clearAuth } from './auth'

interface AppState {
  user: User | null
  setUser: (u: User | null) => void
  logout: () => void

  activeRunId: string | null
  setActiveRunId: (id: string | null) => void

  runs: Run[]
  setRuns: (runs: Run[]) => void
  upsertRun: (run: Run) => void

  messages: Record<string, Message[]>
  setMessages: (runId: string, msgs: Message[]) => void
  appendMessage: (msg: Message) => void

  proposals: ActionProposal[]
  setProposals: (p: ActionProposal[]) => void
  removeProposal: (id: string) => void
}

export const useStore = create<AppState>((set) => ({
  // Always start null so SSR and the first client render match.
  // App layout hydrates from localStorage after mount.
  user: null,
  setUser: (user) => set({ user }),
  logout: () => {
    clearAuth()
    set({ user: null })
  },

  activeRunId: null,
  setActiveRunId: (id) => set({ activeRunId: id }),

  runs: [],
  setRuns: (runs) => set({ runs }),
  upsertRun: (run) =>
    set((s) => {
      const idx = s.runs.findIndex((r) => r.id === run.id)
      if (idx >= 0) {
        const next = [...s.runs]
        next[idx] = run
        return { runs: next }
      }
      return { runs: [run, ...s.runs] }
    }),

  messages: {},
  setMessages: (runId, msgs) =>
    set((s) => ({ messages: { ...s.messages, [runId]: msgs } })),
  appendMessage: (msg) =>
    set((s) => {
      const existing = s.messages[msg.run_id] ?? []
      const idx = existing.findIndex((m) => m.id === msg.id)
      if (idx >= 0) return s
      return { messages: { ...s.messages, [msg.run_id]: [...existing, msg] } }
    }),

  proposals: [],
  setProposals: (proposals) => set({ proposals }),
  removeProposal: (id) =>
    set((s) => ({ proposals: s.proposals.filter((p) => p.id !== id) })),
}))
