'use client'

import { useEffect, useState } from 'react'
import { listProposals, approveProposal, rejectProposal } from '@/lib/api'
import type { ActionProposal } from '@/lib/types'
import { formatRelative } from '@/lib/utils'
import { ShieldCheck, Check, X, Loader2, AlertTriangle } from 'lucide-react'

function ProposalCard({
  proposal,
  onApprove,
  onReject,
  busy,
}: {
  proposal: ActionProposal
  onApprove: () => void
  onReject: () => void
  busy: boolean
}) {
  const [paramsOpen, setParamsOpen] = useState(false)

  return (
    <div className="bg-white rounded-xl border border-amber-200 p-5">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <AlertTriangle size={14} className="text-amber-500 flex-shrink-0" />
            <span className="text-sm font-semibold text-slate-800">{proposal.tool_name}</span>
            <span className="text-xs text-slate-400">{formatRelative(proposal.created_at)}</span>
          </div>
          <p className="text-sm text-slate-600 mb-3">{proposal.description}</p>

          <button
            onClick={() => setParamsOpen(!paramsOpen)}
            className="text-xs text-slate-500 hover:text-slate-700 underline"
          >
            {paramsOpen ? 'Hide' : 'Show'} parameters
          </button>
          {paramsOpen && (
            <pre className="mt-2 p-3 bg-slate-900 text-green-300 rounded-lg text-xs font-mono overflow-x-auto">
              {JSON.stringify(proposal.params, null, 2)}
            </pre>
          )}
        </div>

        <div className="flex gap-2 flex-shrink-0">
          <button
            onClick={onReject}
            disabled={busy}
            className="flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg border border-red-200 text-red-600 hover:bg-red-50 transition-colors disabled:opacity-50"
          >
            <X size={13} /> Reject
          </button>
          <button
            onClick={onApprove}
            disabled={busy}
            className="flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg bg-green-600 text-white hover:bg-green-700 transition-colors disabled:opacity-50"
          >
            {busy ? <Loader2 size={13} className="animate-spin" /> : <Check size={13} />}
            Approve
          </button>
        </div>
      </div>
    </div>
  )
}

export default function ProposalsPage() {
  const [proposals, setProposals] = useState<ActionProposal[]>([])
  const [loading, setLoading] = useState(true)
  const [busyId, setBusyId] = useState<string | null>(null)

  async function load() {
    setLoading(true)
    listProposals()
      .then((p) => setProposals(p ?? []))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  async function handleApprove(id: string) {
    setBusyId(id)
    try {
      await approveProposal(id)
      setProposals((prev) => prev.filter((p) => p.id !== id))
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed')
    } finally {
      setBusyId(null)
    }
  }

  async function handleReject(id: string) {
    setBusyId(id)
    try {
      await rejectProposal(id)
      setProposals((prev) => prev.filter((p) => p.id !== id))
    } finally {
      setBusyId(null)
    }
  }

  return (
    <div className="p-6 max-w-3xl mx-auto">
      <div className="flex items-center gap-3 mb-6">
        <ShieldCheck size={20} className="text-amber-500" />
        <div>
          <h1 className="text-xl font-bold text-slate-900">Pending Approvals</h1>
          <p className="text-sm text-slate-500">Agent-requested write actions requiring PM sign-off</p>
        </div>
      </div>

      {loading ? (
        <div className="space-y-3">
          {[...Array(2)].map((_, i) => <div key={i} className="h-28 bg-slate-100 rounded-xl animate-pulse" />)}
        </div>
      ) : proposals.length === 0 ? (
        <div className="text-center py-16 text-slate-400">
          <ShieldCheck size={32} className="mx-auto mb-3 opacity-30" />
          <p className="font-medium">No pending approvals</p>
          <p className="text-sm mt-1">All clear — no write actions waiting for review.</p>
        </div>
      ) : (
        <div className="space-y-4">
          {proposals.map((p) => (
            <ProposalCard
              key={p.id}
              proposal={p}
              onApprove={() => handleApprove(p.id)}
              onReject={() => handleReject(p.id)}
              busy={busyId === p.id}
            />
          ))}
        </div>
      )}
    </div>
  )
}
