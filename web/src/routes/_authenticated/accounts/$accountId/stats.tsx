import { createFileRoute } from '@tanstack/react-router'

import { AccountStatsPage } from '@/features/accounts/components/account-stats-page'
import { statisticsSearchSchema } from '@/features/statistics/schema'
import { buildStatisticsSearch } from '@/features/statistics/search'

export const Route = createFileRoute(
  '/_authenticated/accounts/$accountId/stats'
)({
  component: AccountStatsRoute,
  validateSearch: statisticsSearchSchema,
})

function AccountStatsRoute() {
  const { accountId } = Route.useParams()
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <AccountStatsPage
      accountId={accountId}
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={buildStatisticsSearch(rawSearch)}
    />
  )
}
