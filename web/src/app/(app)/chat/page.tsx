'use client'

import { useEffect, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import {
  createRun,
  getAgent,
  getRun,
  listAgents,
  listProjects,
  sendMessage,
  getRunMessages,
  createRunStream,
} from '@/lib/api'
import type { Agent, Message, Project, Run } from '@/lib/types'
import { MessageBubble } from '@/components/chat/MessageBubble'
import { cn } from '@/lib/utils'
import { Send, Loader2, ChevronDown } from 'lucide-react'

export default function ChatPage() {
  const router = useRouter()
  const [projects, setProjects] = useState<Project[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [selectedProject, setSelectedProject] = useState<string>('')
  const [selectedAgent, setSelectedAgent] = useState<string>('')
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [run, setRun] = useState<Run | null>(null)
  const [loading, setLoading] = useState(false)
  const [streaming, setStreaming] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [title, setTitle] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  // Synchronous guards: React state updates are async, so `loading`/`streaming`
  // can't prevent a rapid double-Enter in the same tick.
  const sendingRef = useRef(false)
  // Track the active SSE stream so it is always closed on unmount/failure.
  const stopStreamRef = useRef<(() => void) | null>(null)

  useEffect(() => {
    return () => stopStreamRef.current?.()
  }, [])

  useEffect(() => {
    listProjects()
      .then((ps) => {
        setProjects(ps ?? [])
        if (ps?.[0]) setSelectedProject(ps[0].id)
      })
      .catch((err) => setError(err.message))
  }, [])

  useEffect(() => {
    if (!selectedProject) return
    listAgents(selectedProject)
      .then((as) => {
        setAgents(as ?? [])
        if (as?.[0]) setSelectedAgent(as[0].id)
      })
      .catch((err) => setError(err.message))
  }, [selectedProject])

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })
  }, [messages])

  // Watchdog: SSE can be dropped (idle proxy, network blip) while the run keeps
  // processing server-side. While streaming, poll run status + messages so the
  // "Thinking" state always ends and the final answer is reconciled.
  useEffect(() => {
    if (!streaming || !run) return
    const timer = setInterval(async () => {
      try {
        const [runStatus, msgs] = await Promise.all([
          getRun(run.id),
          getRunMessages(run.id),
        ])
        msgs.forEach(appendMsg)
        if (runStatus.status === 'completed' || runStatus.status === 'failed') {
          setStreaming(false)
          stopStreamRef.current?.()
          stopStreamRef.current = null
          if (runStatus.status === 'failed') {
            setError('Run failed while the connection was down.')
          }
        }
      } catch {
        // transient — keep polling
      }
    }, 3000)
    return () => clearInterval(timer)
  }, [streaming, run])

  function appendMsg(msg: Message) {
    setMessages((prev) => {
      if (prev.find((m) => m.id === msg.id)) return prev

      // Replace the temporary bubble once the persisted user message arrives
      // over SSE. This keeps sending instant without rendering it twice.
      if (msg.role === 'user') {
        const optimisticIndex = prev.findIndex(
          (m) =>
            m.role === 'user' &&
            m.id.startsWith('optimistic-') &&
            m.content === msg.content,
        )
        if (optimisticIndex !== -1) {
          const next = [...prev]
          next[optimisticIndex] = msg
          return next
        }
      }

      return [...prev, msg]
    })
  }

  async function startRun(firstMessage: string) {
    if (!selectedAgent) {
      setError('No agent selected. Please wait for agents to load or refresh the page.')
      setStreaming(false)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const newRun = await createRun({
        agent_id: selectedAgent,
        title: title || firstMessage.slice(0, 60),
      })
      setRun(newRun)

      // Start streaming before sending message so we don't miss events
      setStreaming(true)
      stopStreamRef.current = createRunStream(
        newRun.id,
        (ev) => {
          if (ev.type === 'message') appendMsg(ev.payload as Message)
        },
        () => { setStreaming(false); stopStreamRef.current = null },
        (err) => { setStreaming(false); stopStreamRef.current = null; setError(err.message) },
      )

      await sendMessage(newRun.id, firstMessage)
      setLoading(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
      setLoading(false)
      setStreaming(false)
      stopStreamRef.current?.()
      stopStreamRef.current = null
    }
  }

  async function continueRun(msg: string) {
    if (!run) return
    setStreaming(true)
    setError(null)
    stopStreamRef.current = createRunStream(
      run.id,
      (ev) => {
        if (ev.type === 'message') appendMsg(ev.payload as Message)
      },
      () => { setStreaming(false); stopStreamRef.current = null },
      (err) => { setStreaming(false); stopStreamRef.current = null; setError(err.message) },
    )
    try {
      await sendMessage(run.id, msg)
    } catch (err) {
      setStreaming(false)
      stopStreamRef.current?.()
      stopStreamRef.current = null
      setError(err instanceof Error ? err.message : 'Something went wrong')
    }
  }

  async function handleSend() {
    const text = input.trim()
    if (!text || loading || streaming || sendingRef.current) return
    sendingRef.current = true
    setInput('')
    setError(null)
    setStreaming(true)
    appendMsg({
      id: `optimistic-${crypto.randomUUID()}`,
      run_id: run?.id ?? '',
      role: 'user',
      content: text,
      created_at: new Date().toISOString(),
    })

    try {
      if (!run) {
        await startRun(text)
      } else {
        await continueRun(text)
      }
    } finally {
      sendingRef.current = false
      setTimeout(() => inputRef.current?.focus(), 100)
    }
  }

  function handleKey(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const agent = agents.find((a) => a.id === selectedAgent)
  const lastUserMessageIndex = messages.findLastIndex((message) => message.role === 'user')

  return (
    <div className="flex flex-col h-screen">
      {/* Top bar */}
      <div className="flex items-center gap-3 px-4 py-3 border-b border-slate-200 bg-white">
        <div className="flex-1 flex items-center gap-3">
          {/* Project selector */}
          <div className="relative">
            <select
              value={selectedProject}
              onChange={(e) => setSelectedProject(e.target.value)}
              disabled={!!run}
              className="appearance-none pl-3 pr-8 py-1.5 text-sm rounded-lg border border-slate-200 bg-white focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
            >
              {projects.map((p) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
            <ChevronDown size={13} className="absolute right-2.5 top-2.5 text-slate-400 pointer-events-none" />
          </div>

          {/* Agent selector */}
          <div className="relative">
            <select
              value={selectedAgent}
              onChange={(e) => setSelectedAgent(e.target.value)}
              disabled={!!run}
              className="appearance-none pl-3 pr-8 py-1.5 text-sm rounded-lg border border-slate-200 bg-white focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
            >
              {agents.map((a) => (
                <option key={a.id} value={a.id}>{a.name}</option>
              ))}
            </select>
            <ChevronDown size={13} className="absolute right-2.5 top-2.5 text-slate-400 pointer-events-none" />
          </div>

          {agent && (
            <span className="text-xs text-slate-400 border-l border-slate-200 pl-3">
              {agent.default_provider} · {agent.default_model}
            </span>
          )}
        </div>

        {run && (
          <button
            onClick={() => router.push(`/runs/${run.id}`)}
            className="text-xs text-blue-600 hover:text-blue-700"
          >
            View run →
          </button>
        )}
      </div>

      {/* Messages */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto px-6 py-6 space-y-4">
        {messages.length === 0 && (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <div className="w-16 h-16 rounded-2xl bg-blue-100 flex items-center justify-center mb-4">
              <span className="text-2xl">🔍</span>
            </div>
            <h2 className="font-semibold text-slate-700 mb-1">Start an investigation</h2>
            <p className="text-sm text-slate-400 max-w-sm">
              Ask about card registration failures, stuck rows, PSP errors, or any pipeline issue.
            </p>
            <div className="mt-6 grid grid-cols-1 gap-2 w-full max-w-sm">
              {[
                'Why is Melli card registration failing this morning?',
                'How many cards are stuck in add_in_progress?',
                'Show me the distribution of Mellat response codes in the last 24h',
              ].map((suggestion) => (
                <button
                  key={suggestion}
                  onClick={() => setInput(suggestion)}
                  className="text-left text-sm text-slate-600 bg-white border border-slate-200 px-3 py-2 rounded-lg hover:border-blue-300 hover:text-blue-700 transition-colors"
                >
                  {suggestion}
                </button>
              ))}
            </div>
          </div>
        )}
        {messages.map((msg, index) => (
          <div key={msg.id}>
            <MessageBubble message={msg} />
            {streaming && index === lastUserMessageIndex && (
              <div
                role="status"
                aria-live="polite"
                className="animate-chat-message mt-4 flex items-center gap-3"
              >
                <div className="relative flex h-8 w-8 flex-shrink-0 items-center justify-center">
                  <span className="absolute inset-0 rounded-full bg-blue-200/70 animate-thinking-ping" />
                  <div className="relative flex h-8 w-8 items-center justify-center rounded-full border border-blue-100 bg-gradient-to-br from-blue-50 to-indigo-100 shadow-sm">
                    <Loader2 size={14} className="animate-spin text-blue-600" />
                  </div>
                </div>
                <div className="flex items-center gap-2 rounded-2xl rounded-tl-sm border border-slate-200/80 bg-white/90 px-4 py-3 shadow-sm backdrop-blur">
                  <span className="text-sm font-medium text-slate-500">Thinking</span>
                  <span className="flex items-center gap-1" aria-hidden="true">
                    <span className="thinking-dot" />
                    <span className="thinking-dot" />
                    <span className="thinking-dot" />
                  </span>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Error banner */}
      {error && (
        <div className="mx-4 mb-2 px-4 py-2 bg-red-50 border border-red-200 rounded-lg flex items-center justify-between">
          <span className="text-sm text-red-700">{error}</span>
          <button onClick={() => setError(null)} className="text-red-400 hover:text-red-600 ml-3 text-lg leading-none">&times;</button>
        </div>
      )}

      {/* Input */}
      <div className="border-t border-slate-200 bg-white px-4 py-3">
        {!run && (
          <input
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Investigation title (optional)"
            className="w-full text-xs text-slate-500 px-0 py-1 mb-2 border-none outline-none bg-transparent placeholder:text-slate-300"
          />
        )}
        <div className="flex items-end gap-3 bg-white border border-slate-200 rounded-xl p-3 focus-within:ring-2 focus-within:ring-blue-500 focus-within:border-transparent transition-all">
          <textarea
            ref={inputRef}
            rows={1}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKey}
            placeholder="Ask about card registrations, failures, stuck rows…"
            className="flex-1 resize-none text-sm text-slate-800 outline-none placeholder:text-slate-400 max-h-32 overflow-y-auto leading-relaxed"
          />
          <button
            onClick={handleSend}
            disabled={!input.trim() || loading || streaming}
            className={cn(
              'flex-shrink-0 w-8 h-8 rounded-lg flex items-center justify-center transition-colors',
              input.trim() && !loading && !streaming
                ? 'bg-blue-600 hover:bg-blue-700 text-white'
                : 'bg-slate-100 text-slate-400 cursor-not-allowed',
            )}
          >
            {loading || streaming
              ? <Loader2 size={15} className="animate-spin" />
              : <Send size={15} />
            }
          </button>
        </div>
        <p className="text-xs text-slate-400 mt-2 px-1">
          Press Enter to send · Shift+Enter for newline
        </p>
      </div>
    </div>
  )
}
