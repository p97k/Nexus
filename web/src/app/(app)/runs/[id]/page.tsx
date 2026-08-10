'use client'

import { useEffect, useState } from 'react'
import { useParams } from 'next/navigation'
import Link from 'next/link'
import { getRun, getRunMessages } from '@/lib/api'
import type { Run, Message } from '@/lib/types'
import { MessageBubble } from '@/components/chat/MessageBubble'
import { formatRelative, formatTokens } from '@/lib/utils'
import { ArrowLeft, Coins, Layers, Clock } from 'lucide-react'

export default function RunDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [run, setRun] = useState<Run | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([getRun(id), getRunMessages(id)])
      .then(([r, msgs]) => {
        setRun(r)
        setMessages(msgs ?? [])
      })
      .finally(() => setLoading(false))
  }, [id])

  if (loading) {
    return (
      <div className="p-6 space-y-4">
        <div className="h-8 w-48 bg-slate-100 rounded animate-pulse" />
        <div className="h-4 w-96 bg-slate-100 rounded animate-pulse" />
        <div className="space-y-3 mt-6">
          {[...Array(4)].map((_, i) => <div key={i} className="h-16 bg-slate-100 rounded-xl animate-pulse" />)}
        </div>
      </div>
    )
  }

  if (!run) {
    return <div className="p-6 text-slate-500">Run not found.</div>
  }

  return (
    <div className="p-6 max-w-3xl mx-auto">
      {/* Header */}
      <Link href="/runs" className="inline-flex items-center gap-1.5 text-sm text-slate-500 hover:text-slate-700 mb-4">
        <ArrowLeft size={14} /> Back to runs
      </Link>

      <h1 className="text-lg font-bold text-slate-900 mb-1">{run.title}</h1>

      <div className="flex flex-wrap items-center gap-4 mb-6 text-xs text-slate-500">
        <span className="flex items-center gap-1"><Clock size={12} /> {formatRelative(run.created_at)}</span>
        <span>{run.provider} · {run.model}</span>
        <span className="flex items-center gap-1"><Layers size={12} /> {run.step_count} steps</span>
        <span className="flex items-center gap-1">
          <Coins size={12} />
          {formatTokens(run.tokens_in)} in / {formatTokens(run.tokens_out)} out
        </span>
        <span className={
          run.status === 'completed' ? 'text-green-600 font-medium' :
          run.status === 'failed' ? 'text-red-600 font-medium' :
          run.status === 'running' ? 'text-blue-600 font-medium' :
          'text-amber-600 font-medium'
        }>● {run.status}</span>
      </div>

      {/* Messages */}
      <div className="space-y-4">
        {messages.map((msg) => (
          <MessageBubble key={msg.id} message={msg} />
        ))}
      </div>

      {messages.length === 0 && (
        <div className="text-center py-12 text-slate-400 text-sm">No messages in this run.</div>
      )}
    </div>
  )
}
