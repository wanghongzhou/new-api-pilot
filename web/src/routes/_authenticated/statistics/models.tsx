import { createFileRoute } from '@tanstack/react-router'

import { StatisticsPage } from '@/features/statistics/components/statistics-page'
import { statisticsSearchSchema } from '@/features/statistics/schema'
import { buildStatisticsSearch } from '@/features/statistics/search'

export const Route = createFileRoute('/_authenticated/statistics/models')({
  component: ModelStatisticsRoute,
  validateSearch: statisticsSearchSchema,
})

function ModelStatisticsRoute() {
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <StatisticsPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      scope='model'
      search={buildStatisticsSearch(rawSearch)}
    />
  )
}
