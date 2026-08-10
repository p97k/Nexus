'use client'

import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { cn } from '@/lib/utils'
import { useStore } from '@/lib/store'
import {
  LayoutDashboard,
  MessageSquare,
  History,
  ShieldCheck,
  Settings,
  LogOut,
  Zap,
} from 'lucide-react'

const nav = [
  { label: 'Dashboard', href: '/dashboard', icon: LayoutDashboard },
  { label: 'New Chat', href: '/chat', icon: MessageSquare },
  { label: 'Run History', href: '/runs', icon: History },
  { label: 'Approvals', href: '/proposals', icon: ShieldCheck },
  { label: 'Settings', href: '/settings', icon: Settings },
]

export function Sidebar() {
  const pathname = usePathname()
  const router = useRouter()
  const { user, logout } = useStore()

  function handleLogout() {
    logout()
    router.push('/login')
  }

  return (
    <aside className="fixed inset-y-0 left-0 z-40 flex flex-col w-60 bg-slate-900 text-slate-100">
      {/* Brand */}
      <div className="flex items-center gap-2.5 px-4 h-16 border-b border-slate-800">
        <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-blue-600">
          <Zap size={16} className="text-white" />
        </div>
        <span className="font-bold text-base tracking-tight">Nexus</span>
      </div>

      {/* Nav */}
      <nav className="flex-1 py-4 px-2 space-y-0.5 overflow-y-auto">
        {nav.map(({ label, href, icon: Icon }) => {
          const active = href === '/dashboard'
            ? pathname === '/dashboard'
            : pathname.startsWith(href)
          return (
            <Link
              key={href}
              href={href}
              className={cn(
                'flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors',
                active
                  ? 'bg-blue-600 text-white'
                  : 'text-slate-400 hover:text-white hover:bg-slate-800',
              )}
            >
              <Icon size={17} />
              {label}
            </Link>
          )
        })}
      </nav>

      {/* User */}
      <div className="border-t border-slate-800 p-3">
        <div className="flex items-center gap-3 px-2 py-2">
          <div className="w-8 h-8 rounded-full bg-blue-600 flex items-center justify-center text-xs font-bold text-white flex-shrink-0">
            {user?.name?.[0]?.toUpperCase() ?? 'U'}
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-xs font-medium text-white truncate">{user?.name ?? 'User'}</p>
            <p className="text-xs text-slate-500 truncate">{user?.role}</p>
          </div>
          <button
            onClick={handleLogout}
            className="text-slate-500 hover:text-slate-300 transition-colors"
            title="Sign out"
          >
            <LogOut size={15} />
          </button>
        </div>
      </div>
    </aside>
  )
}
