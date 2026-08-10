'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { getStoredUser, isAuthenticated } from '@/lib/auth'
import { useStore } from '@/lib/store'
import { Sidebar } from '@/components/layout/Sidebar'

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const setUser = useStore((s) => s.setUser)

  useEffect(() => {
    if (!isAuthenticated()) {
      router.replace('/login')
      return
    }
    // Hydrate after mount so SSR HTML matches the first client render.
    setUser(getStoredUser())
  }, [router, setUser])

  return (
    <div className="flex h-screen bg-slate-50">
      <Sidebar />
      <main className="flex-1 ml-60 overflow-y-auto">
        {children}
      </main>
    </div>
  )
}
