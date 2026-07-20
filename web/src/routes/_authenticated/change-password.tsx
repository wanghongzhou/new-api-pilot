import { createFileRoute } from '@tanstack/react-router'

import { ChangePasswordPage } from '@/features/auth/components/change-password-page'

export const Route = createFileRoute('/_authenticated/change-password')({
  component: ChangePasswordPage,
})
