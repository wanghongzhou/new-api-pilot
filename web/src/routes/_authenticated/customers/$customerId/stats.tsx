import { createFileRoute } from '@tanstack/react-router'

import { CustomerStatsPage } from '@/features/customers/components/customer-stats-page'
import { statisticsSearchSchema } from '@/features/statistics/schema'
import { buildStatisticsSearch } from '@/features/statistics/search'

export const Route = createFileRoute(
  '/_authenticated/customers/$customerId/stats'
)({
  component: CustomerStatsRoute,
  validateSearch: statisticsSearchSchema,
})

function CustomerStatsRoute() {
  const { customerId } = Route.useParams()
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <CustomerStatsPage
      customerId={customerId}
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={buildStatisticsSearch(rawSearch)}
    />
  )
}
