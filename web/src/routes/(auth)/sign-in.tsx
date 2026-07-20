import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'

import { SignInPage } from '@/features/auth/components/sign-in-page'
import { useAuthStore } from '@/stores/auth-store'

const signInSearchSchema = z.object({
  redirect: z.string().max(2048).optional().catch(undefined),
})

export const Route = createFileRoute('/(auth)/sign-in')({
  beforeLoad: () => {
    if (useAuthStore.getState().user) throw redirect({ to: '/dashboard' })
  },
  component: SignInRoute,
  validateSearch: signInSearchSchema,
})

function SignInRoute() {
  const { redirect: redirectPath } = Route.useSearch()
  return <SignInPage redirect={redirectPath} />
}
