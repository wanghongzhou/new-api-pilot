import { createFileRoute } from '@tanstack/react-router'

import { StatisticsPage } from '@/features/statistics/components/statistics-page'
import { statisticsSearchSchema } from '@/features/statistics/schema'
import { buildStatisticsSearch } from '@/features/statistics/search'

export const Route = createFileRoute('/_authenticated/statistics/customers')({
  component: CustomerStatisticsRoute,
  validateSearch: statisticsSearchSchema,
})

function CustomerStatisticsRoute() {
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <StatisticsPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      scope='customer'
      search={buildStatisticsSearch(rawSearch)}
    />
  )
}
