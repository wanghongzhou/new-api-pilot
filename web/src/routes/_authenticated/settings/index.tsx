import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/settings/')({
  beforeLoad: () => {
    throw redirect({ replace: true, to: '/settings/system' })
  },
})
