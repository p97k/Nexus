'use client'

import { useState } from 'react'
import type { Message } from '@/lib/types'
import { normalizeToolCalls } from '@/lib/types'
import { cn } from '@/lib/utils'
import { ChevronDown, ChevronRight, Wrench, User, Bot } from 'lucide-react'

function ToolCallBlock({ name, input }: { name: string; input: unknown }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="mt-2 border border-slate-200 rounded-lg overflow-hidden">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 w-full px-3 py-2 bg-slate-50 text-xs text-slate-600 hover:bg-slate-100 transition-colors text-left"
      >
        <Wrench size={12} />
        <span className="font-mono font-medium">{name}</span>
        <span className="ml-auto">{open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}</span>
      </button>
      {open && (
        <pre className="animate-chat-expand px-3 py-2 text-xs bg-slate-900 text-green-300 font-mono overflow-x-auto">
          {JSON.stringify(input, null, 2)}
        </pre>
      )}
    </div>
  )
}

function ToolResultBubble({ message }: { message: Message }) {
  const [open, setOpen] = useState(false)
  let parsed: unknown = null
  try { parsed = JSON.parse(message.content) } catch { parsed = message.content }

  return (
    <div className="animate-chat-message flex justify-center my-2">
      <div className="max-w-lg w-full border border-slate-200 rounded-lg overflow-hidden text-xs shadow-sm transition-shadow duration-300 hover:shadow-md">
        <button
          onClick={() => setOpen(!open)}
          className="flex items-center gap-2 w-full px-3 py-2 bg-slate-50 text-slate-500 hover:bg-slate-100 transition-colors"
        >
          <Wrench size={12} />
          <span className="font-mono">{message.tool_name}</span>
          <span className="text-slate-400 ml-auto">result {open ? '▲' : '▼'}</span>
        </button>
        {open && (
          <pre className="animate-chat-expand px-3 py-2 bg-slate-900 text-emerald-300 font-mono overflow-x-auto text-xs leading-relaxed">
            {JSON.stringify(parsed, null, 2)}
          </pre>
        )}
      </div>
    </div>
  )
}

interface Props {
  message: Message
}

export function MessageBubble({ message }: Props) {
  if (message.role === 'tool') {
    return <ToolResultBubble message={message} />
  }

  const isUser = message.role === 'user'
  const toolCalls = normalizeToolCalls(message.tool_calls)

  return (
    <div className={cn(
      'animate-chat-message flex gap-3',
      isUser ? 'flex-row-reverse [--chat-enter-x:10px]' : 'flex-row [--chat-enter-x:-10px]',
    )}>
      {/* Avatar */}
      <div className={cn(
        'flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center shadow-sm',
        isUser ? 'bg-blue-600' : 'bg-slate-200',
      )}>
        {isUser
          ? <User size={15} className="text-white" />
          : <Bot size={15} className="text-slate-600" />
        }
      </div>

      {/* Bubble */}
      <div className={cn(
        'max-w-[70%] rounded-2xl px-4 py-3 text-sm leading-relaxed shadow-sm transition-shadow duration-300 hover:shadow-md',
        isUser
          ? 'bg-blue-600 text-white rounded-tr-none'
          : 'bg-white border border-slate-200 text-slate-800 rounded-tl-none',
      )}>
        <p className="whitespace-pre-wrap">{message.content}</p>

        {/* Tool calls the assistant requested */}
        {toolCalls.length > 0 && (
          <div className="mt-2 space-y-1">
            {toolCalls.map((tc) => (
              <ToolCallBlock key={tc.id} name={tc.name} input={tc.input} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
