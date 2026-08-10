'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { getCashbackDashboard, listRuns } from '@/lib/api'
import type { BankStat, Run } from '@/lib/types'
import { formatRelative, formatMinutes } from '@/lib/utils'
import { cn } from '@/lib/utils'
import {
  AlertTriangle,
  CheckCircle2,
  Clock,
  MessageSquare,
  RefreshCw,
  TrendingUp,
} from 'lucide-react'

function StatusBadge({ status }: { status: Run['status'] }) {
  const map = {
    completed: 'bg-green-100 text-green-700',
    running: 'bg-blue-100 text-blue-700',
    pending: 'bg-amber-100 text-amber-700',
    failed: 'bg-red-100 text-red-700',
  }
  return (
    <span className={cn('inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium', map[status])}>
      {status}
    </span>
  )
}

function BankHealthCard({ stat }: { stat: BankStat }) {
  const health = stat.stuck_count > 0 ? 'danger'
    : stat.failed_count > 50 ? 'warning'
    : 'good'

  return (
    <div className={cn(
      'bg-white rounded-xl border p-4 flex flex-col gap-3',
      health === 'danger' ? 'border-red-200' :
      health === 'warning' ? 'border-amber-200' : 'border-slate-200',
    )}>
      <div className="flex items-center justify-between">
        <span className="font-semibold text-slate-800 capitalize">{stat.bank_name}</span>
        {health === 'danger' ? (
          <AlertTriangle size={16} className="text-red-500" />
        ) : health === 'warning' ? (
          <AlertTriangle size={16} className="text-amber-500" />
        ) : (
          <CheckCircle2 size={16} className="text-green-500" />
        )}
      </div>

      <div className="grid grid-cols-3 gap-2 text-center">
        <div>
          <div className="text-lg font-bold text-amber-600">{stat.pending_count}</div>
          <div className="text-xs text-slate-500">Pending</div>
        </div>
        <div>
          <div className="text-lg font-bold text-red-600">{stat.failed_count}</div>
          <div className="text-xs text-slate-500">Failed</div>
        </div>
        <div>
          <div className={cn('text-lg font-bold', stat.stuck_count > 0 ? 'text-red-700' : 'text-slate-400')}>
            {stat.stuck_count}
          </div>
          <div className="text-xs text-slate-500">Stuck</div>
        </div>
      </div>

      {stat.oldest_pending_minutes !== undefined && stat.oldest_pending_minutes > 0 && (
        <div className="flex items-center gap-1.5 text-xs text-slate-500">
          <Clock size={12} />
          Oldest: {formatMinutes(stat.oldest_pending_minutes)} ago
        </div>
      )}
    </div>
  )
}

export default function DashboardPage() {
  const [stats, setStats] = useState<BankStat[]>([])
  const [runs, setRuns] = useState<Run[]>([])
  const [loading, setLoading] = useState(true)
  const [lastRefresh, setLastRefresh] = useState(new Date())
  const [mounted, setMounted] = useState(false)

  async function load() {
    setLoading(true)
    try {
      const [s, r] = await Promise.allSettled([
        getCashbackDashboard(),
        listRuns(),
      ])
      if (s.status === 'fulfilled') setStats(s.value)
      if (r.status === 'fulfilled') setRuns(r.value.slice(0, 5))
      setLastRefresh(new Date())
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { setMounted(true); load() }, [])

  const totalPending = stats.reduce((a, s) => a + s.pending_count, 0)
  const totalFailed = stats.reduce((a, s) => a + s.failed_count, 0)
  const totalStuck = stats.reduce((a, s) => a + s.stuck_count, 0)

  return (
    <div className="p-6 max-w-6xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-xl font-bold text-slate-900">Dashboard</h1>
          <p className="text-sm text-slate-500 mt-0.5">
            Offline Cashback pipeline health
          </p>
        </div>
        <button
          onClick={load}
          disabled={loading}
          className="flex items-center gap-2 text-sm text-slate-600 hover:text-slate-900 bg-white border border-slate-200 px-3 py-1.5 rounded-lg transition-colors disabled:opacity-50"
        >
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>

      {/* Summary row */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        {[
          { label: 'Total Pending', value: totalPending, icon: Clock, color: 'text-amber-600' },
          { label: 'Total Failed', value: totalFailed, icon: AlertTriangle, color: 'text-red-600' },
          { label: 'Stuck in Progress', value: totalStuck, icon: TrendingUp, color: totalStuck > 0 ? 'text-red-700' : 'text-green-600' },
        ].map(({ label, value, icon: Icon, color }) => (
          <div key={label} className="bg-white rounded-xl border border-slate-200 p-4 flex items-center gap-4">
            <div className={cn('text-2xl font-bold', color)}>{value}</div>
            <div>
              <div className="text-sm font-medium text-slate-700">{label}</div>
              <Icon size={14} className={cn('mt-0.5', color)} />
            </div>
          </div>
        ))}
      </div>

      {/* Bank cards */}
      {stats.length > 0 ? (
        <div className="mb-8">
          <h2 className="text-sm font-semibold text-slate-700 uppercase tracking-wide mb-3">
            Bank Health
          </h2>
          <div className="grid grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
            {stats.map((stat) => (
              <BankHealthCard key={stat.bank_name} stat={stat} />
            ))}
          </div>
        </div>
      ) : (
        <div className="mb-8 bg-slate-100 rounded-xl p-8 text-center text-slate-500 text-sm">
          {loading ? 'Loading bank stats…' : 'No data — configure CASHBACK_DB_URL to see live stats'}
        </div>
      )}

      {/* Recent runs + start CTA */}
      <div className="grid grid-cols-5 gap-6">
        {/* Recent runs */}
        <div className="col-span-3 bg-white rounded-xl border border-slate-200">
          <div className="flex items-center justify-between px-4 py-3 border-b border-slate-100">
            <span className="text-sm font-semibold text-slate-700">Recent Investigations</span>
            <Link href="/runs" className="text-xs text-blue-600 hover:text-blue-700">View all</Link>
          </div>
          <div className="divide-y divide-slate-100">
            {runs.length === 0 ? (
              <div className="px-4 py-6 text-sm text-slate-400 text-center">No runs yet</div>
            ) : runs.map((run) => (
              <Link
                key={run.id}
                href={`/runs/${run.id}`}
                className="flex items-center gap-3 px-4 py-3 hover:bg-slate-50 transition-colors"
              >
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium text-slate-800 truncate">{run.title}</div>
                  <div className="text-xs text-slate-500">{formatRelative(run.created_at)}</div>
                </div>
                <StatusBadge status={run.status} />
              </Link>
            ))}
          </div>
        </div>

        {/* Quick start */}
        <div className="col-span-2 bg-gradient-to-br from-blue-600 to-blue-700 rounded-xl p-6 text-white flex flex-col justify-between">
          <div>
            <MessageSquare size={24} className="mb-3 opacity-80" />
            <h3 className="font-semibold text-lg mb-2">Start Investigation</h3>
            <p className="text-sm text-blue-100 leading-relaxed">
              Ask the agent to diagnose card failures, stuck registrations, or PSP errors.
            </p>
          </div>
          <Link
            href="/chat"
            className="mt-4 inline-flex items-center justify-center gap-2 bg-white text-blue-700 px-4 py-2 rounded-lg text-sm font-semibold hover:bg-blue-50 transition-colors"
          >
            New Chat
          </Link>
        </div>
      </div>

      {mounted && (
        <div className="mt-4 text-xs text-slate-400 text-right">
          Last refreshed: {lastRefresh.toLocaleTimeString()}
        </div>
      )}
    </div>
  )
}
