'use client'

import type { User } from './types'

export function saveAuth(token: string, user: User) {
  localStorage.setItem('nexus_token', token)
  localStorage.setItem('nexus_user', JSON.stringify(user))
}

export function clearAuth() {
  localStorage.removeItem('nexus_token')
  localStorage.removeItem('nexus_user')
}

export function getStoredUser(): User | null {
  if (typeof window === 'undefined') return null
  const raw = localStorage.getItem('nexus_user')
  if (!raw) return null
  try {
    return JSON.parse(raw) as User
  } catch {
    return null
  }
}

export function getToken(): string | null {
  if (typeof window === 'undefined') return null
  return localStorage.getItem('nexus_token')
}

export function isAuthenticated(): boolean {
  return !!getToken()
}
