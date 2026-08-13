import { createFileRoute, redirect } from '@tanstack/react-router'

import { ChangePasswordPage } from '@/features/auth/components/change-password-page'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/change-password')({
  beforeLoad: () => {
    if (useAuthStore.getState().user?.must_change_password === false) {
      throw redirect({ replace: true, to: '/dashboard' })
    }
  },
  component: ChangePasswordPage,
})
