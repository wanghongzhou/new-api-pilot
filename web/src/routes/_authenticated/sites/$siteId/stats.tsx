import { createFileRoute } from '@tanstack/react-router'

import { SiteStatsPage } from '@/features/sites/components/site-stats-page'
import { statisticsSearchSchema } from '@/features/statistics/schema'
import { buildStatisticsSearch } from '@/features/statistics/search'

export const Route = createFileRoute('/_authenticated/sites/$siteId/stats')({
  component: SiteStatsRoute,
  validateSearch: statisticsSearchSchema,
})

function SiteStatsRoute() {
  const { siteId } = Route.useParams()
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <SiteStatsPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={buildStatisticsSearch(rawSearch)}
      siteId={siteId}
    />
  )
}
