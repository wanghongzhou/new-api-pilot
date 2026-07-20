import { ApiError } from '@/lib/api'
import { queryClient } from '@/lib/query-client'
import { useAuthStore } from '@/stores/auth-store'

import { getSelf } from './api'
import type { LoginUser } from './types'

let sessionVerified = false
let verificationPromise: Promise<LoginUser> | null = null

export function markSessionVerified(user: LoginUser): void {
  useAuthStore.getState().setUser(user)
  sessionVerified = true
}

export function resetSessionVerification(): void {
  sessionVerified = false
  verificationPromise = null
}

export function clearAuthenticatedSession(): void {
  resetSessionVerification()
  useAuthStore.getState().clearUser()
  queryClient.clear()
}

export async function ensureSessionVerified(): Promise<LoginUser> {
  const storedUser = useAuthStore.getState().user
  if (!storedUser) {
    throw new ApiError('Authentication is required', {
      kind: 'http',
      status: 401,
      code: 'AUTH_REQUIRED',
      requestId: null,
      fieldErrors: null,
    })
  }
  if (sessionVerified) return storedUser
  if (verificationPromise) return verificationPromise

  verificationPromise = getSelf()
    .then((user) => {
      markSessionVerified(user)
      return user
    })
    .finally(() => {
      verificationPromise = null
    })
  return verificationPromise
}
