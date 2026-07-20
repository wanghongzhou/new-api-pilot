import { create } from 'zustand'

import type { LoginUser } from '@/features/auth/types'
import { setAuthenticatedUserId } from '@/lib/api'
import { isIdString } from '@/lib/api-types'

export const AUTH_USER_STORAGE_KEY = 'pilot-auth-user'
export const AUTH_UID_STORAGE_KEY = 'uid'

interface AuthState {
  user: LoginUser | null
  clearUser: () => void
  setUser: (user: LoginUser) => void
}

function isLoginUser(value: unknown): value is LoginUser {
  if (!value || typeof value !== 'object') return false
  const user = value as Partial<LoginUser>
  return (
    isIdString(user.id) &&
    typeof user.username === 'string' &&
    typeof user.display_name === 'string' &&
    (user.role === 'admin' || user.role === 'viewer') &&
    (user.status === 1 || user.status === 2) &&
    typeof user.must_change_password === 'boolean'
  )
}

function readStoredUser(): LoginUser | null {
  if (typeof window === 'undefined') return null
  try {
    const stored = window.localStorage.getItem(AUTH_USER_STORAGE_KEY)
    if (!stored) return null
    const parsed: unknown = JSON.parse(stored)
    if (isLoginUser(parsed)) {
      if (window.localStorage.getItem(AUTH_UID_STORAGE_KEY) !== parsed.id) {
        window.localStorage.setItem(AUTH_UID_STORAGE_KEY, parsed.id)
      }
      return parsed
    }
  } catch {
    // Invalid local state is cleared below.
  }
  window.localStorage.removeItem(AUTH_USER_STORAGE_KEY)
  window.localStorage.removeItem(AUTH_UID_STORAGE_KEY)
  return null
}

function persistUser(user: LoginUser | null): void {
  setAuthenticatedUserId(user?.id ?? null)
  if (typeof window === 'undefined') return
  if (user) {
    window.localStorage.setItem(AUTH_USER_STORAGE_KEY, JSON.stringify(user))
    window.localStorage.setItem(AUTH_UID_STORAGE_KEY, user.id)
  } else {
    window.localStorage.removeItem(AUTH_USER_STORAGE_KEY)
    window.localStorage.removeItem(AUTH_UID_STORAGE_KEY)
  }
}

const initialUser = readStoredUser()
setAuthenticatedUserId(initialUser?.id ?? null)

export const useAuthStore = create<AuthState>()((set) => ({
  user: initialUser,
  clearUser: () => {
    persistUser(null)
    set({ user: null })
  },
  setUser: (user) => {
    persistUser(user)
    set({ user })
  },
}))
