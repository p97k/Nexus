'use client'

import { useEffect, useState } from 'react'
import { listProviders } from '@/lib/api'
import { useStore } from '@/lib/store'
import { Settings, CheckCircle2, XCircle } from 'lucide-react'

export default function SettingsPage() {
  const user = useStore((s) => s.user)
  const [providers, setProviders] = useState<string[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    listProviders()
      .then((p) => setProviders(p ?? []))
      .finally(() => setLoading(false))
  }, [])

  const allProviders = ['openai', 'anthropic', 'google']

  return (
    <div className="p-6 max-w-2xl mx-auto">
      <div className="flex items-center gap-3 mb-6">
        <Settings size={20} className="text-slate-500" />
        <h1 className="text-xl font-bold text-slate-900">Settings</h1>
      </div>

      {/* Profile */}
      <section className="bg-white rounded-xl border border-slate-200 p-5 mb-4">
        <h2 className="font-semibold text-slate-700 mb-4">Profile</h2>
        <div className="space-y-3">
          <div className="flex justify-between text-sm">
            <span className="text-slate-500">Name</span>
            <span className="font-medium text-slate-800">{user?.name}</span>
          </div>
          <div className="flex justify-between text-sm">
            <span className="text-slate-500">Email</span>
            <span className="font-medium text-slate-800">{user?.email}</span>
          </div>
          <div className="flex justify-between text-sm">
            <span className="text-slate-500">Role</span>
            <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-700">
              {user?.role}
            </span>
          </div>
        </div>
      </section>

      {/* LLM Providers */}
      <section className="bg-white rounded-xl border border-slate-200 p-5 mb-4">
        <h2 className="font-semibold text-slate-700 mb-1">LLM Providers</h2>
        <p className="text-xs text-slate-400 mb-4">
          Providers are configured via server-side environment variables.
        </p>
        {loading ? (
          <div className="space-y-2">
            {allProviders.map((p) => (
              <div key={p} className="h-10 bg-slate-100 rounded-lg animate-pulse" />
            ))}
          </div>
        ) : (
          <div className="space-y-2">
            {allProviders.map((p) => {
              const active = providers.includes(p)
              return (
                <div key={p} className="flex items-center justify-between px-3 py-2.5 rounded-lg border border-slate-100 bg-slate-50">
                  <span className="text-sm font-medium text-slate-700 capitalize">{p}</span>
                  <div className={`flex items-center gap-1.5 text-xs font-medium ${active ? 'text-green-600' : 'text-slate-400'}`}>
                    {active
                      ? <><CheckCircle2 size={14} /> Configured</>
                      : <><XCircle size={14} /> Not configured</>
                    }
                  </div>
                </div>
              )
            })}
          </div>
        )}
        <p className="mt-3 text-xs text-slate-400">
          Set <code className="bg-slate-100 px-1 py-0.5 rounded">OPENAI_API_KEY</code>,{' '}
          <code className="bg-slate-100 px-1 py-0.5 rounded">ANTHROPIC_API_KEY</code>, or{' '}
          <code className="bg-slate-100 px-1 py-0.5 rounded">GOOGLE_API_KEY</code> in your <code className="bg-slate-100 px-1 py-0.5 rounded">.env</code> and restart the server.
        </p>
      </section>

      {/* Cashback adapter */}
      <section className="bg-white rounded-xl border border-slate-200 p-5">
        <h2 className="font-semibold text-slate-700 mb-1">Offline Cashback Adapter</h2>
        <p className="text-xs text-slate-400 mb-4">
          Connection to the cashback PostgreSQL database for agent tools.
        </p>
        <div className="space-y-2 text-sm">
          {[
            { key: 'CASHBACK_ADAPTER_MODE', note: '"db" or "http"' },
            { key: 'CASHBACK_DB_URL', note: 'Read-only Postgres connection string' },
            { key: 'CASHBACK_API_URL', note: 'If mode=http: Laravel Ops API base URL' },
            { key: 'CASHBACK_API_KEY', note: 'If mode=http: API bearer key' },
          ].map(({ key, note }) => (
            <div key={key} className="flex items-start gap-3">
              <code className="text-xs bg-slate-100 px-1.5 py-0.5 rounded text-slate-700 flex-shrink-0">{key}</code>
              <span className="text-xs text-slate-400">{note}</span>
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}
