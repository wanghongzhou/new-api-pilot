import { createFileRoute } from '@tanstack/react-router'

import { balanceSearch } from '@/features/balance-monitor/data'
import { BalanceMonitorPage } from '@/features/balance-monitor/page'

export const Route = createFileRoute('/_authenticated/balance-monitor')({
  validateSearch: balanceSearch,
  component: BalanceRoute,
})

function BalanceRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <BalanceMonitorPage
      search={search}
      onChange={(value) => void navigate({ search: value })}
    />
  )
}
