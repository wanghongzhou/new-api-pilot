import { createFileRoute, redirect } from '@tanstack/react-router'

import { AuthenticatedLayout } from '@/components/layout/authenticated-layout'
import {
  AuthErrorState,
  AuthPendingState,
} from '@/features/auth/components/auth-boundary-state'
import {
  clearAuthenticatedSession,
  ensureSessionVerified,
} from '@/features/auth/session'
import { normalizeApiError } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: async ({ location }) => {
    if (!useAuthStore.getState().user) {
      throw redirect({
        search: { redirect: location.href },
        to: '/sign-in',
      })
    }

    let user
    try {
      user = await ensureSessionVerified()
    } catch (error) {
      const apiError = normalizeApiError(error)
      if (apiError.status === 401) {
        clearAuthenticatedSession()
        throw redirect({
          search: { redirect: location.href },
          to: '/sign-in',
        })
      }
      throw apiError
    }

    const changingPassword = location.pathname === '/change-password'
    if (user.must_change_password && !changingPassword) {
      throw redirect({ to: '/change-password' })
    }
  },
  component: AuthenticatedLayout,
  errorComponent: AuthErrorState,
  pendingComponent: AuthPendingState,
})
