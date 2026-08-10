'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { listRuns } from '@/lib/api'
import type { Run } from '@/lib/types'
import { formatRelative, cn } from '@/lib/utils'
import { MessageSquare, Clock, Coins } from 'lucide-react'

function StatusDot({ status }: { status: Run['status'] }) {
  const colors = {
    completed: 'bg-green-500',
    running: 'bg-blue-500 animate-pulse',
    pending: 'bg-amber-400',
    failed: 'bg-red-500',
  }
  return <span className={cn('inline-block w-2 h-2 rounded-full', colors[status])} />
}

export default function RunsPage() {
  const [runs, setRuns] = useState<Run[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    listRuns()
      .then((r) => setRuns(r ?? []))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-bold text-slate-900">Run History</h1>
        <Link
          href="/chat"
          className="text-sm bg-blue-600 text-white px-3 py-1.5 rounded-lg hover:bg-blue-700 transition-colors"
        >
          + New
        </Link>
      </div>

      {loading ? (
        <div className="space-y-2">
          {[...Array(5)].map((_, i) => (
            <div key={i} className="h-16 bg-slate-100 rounded-xl animate-pulse" />
          ))}
        </div>
      ) : runs.length === 0 ? (
        <div className="text-center py-16 text-slate-400">
          <MessageSquare size={32} className="mx-auto mb-3 opacity-30" />
          <p className="font-medium">No investigations yet</p>
          <p className="text-sm mt-1">Start a chat to create your first run.</p>
        </div>
      ) : (
        <div className="bg-white rounded-xl border border-slate-200 divide-y divide-slate-100">
          {runs.map((run) => (
            <Link
              key={run.id}
              href={`/runs/${run.id}`}
              className="flex items-center gap-4 px-4 py-3.5 hover:bg-slate-50 transition-colors group"
            >
              <StatusDot status={run.status} />
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium text-slate-800 truncate group-hover:text-blue-600 transition-colors">
                  {run.title}
                </div>
                <div className="flex items-center gap-3 mt-0.5">
                  <span className="text-xs text-slate-400 flex items-center gap-1">
                    <Clock size={11} />
                    {formatRelative(run.created_at)}
                  </span>
                  <span className="text-xs text-slate-400">{run.provider} · {run.model}</span>
                  {run.tokens_in > 0 && (
                    <span className="text-xs text-slate-400 flex items-center gap-1">
                      <Coins size={11} />
                      {run.tokens_in + run.tokens_out} tokens
                    </span>
                  )}
                </div>
              </div>
              <span className={cn(
                'text-xs px-2 py-0.5 rounded-full font-medium flex-shrink-0',
                run.status === 'completed' ? 'bg-green-100 text-green-700' :
                run.status === 'running' ? 'bg-blue-100 text-blue-700' :
                run.status === 'pending' ? 'bg-amber-100 text-amber-700' :
                'bg-red-100 text-red-700',
              )}>
                {run.status}
              </span>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
