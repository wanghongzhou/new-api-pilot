import { createFileRoute } from '@tanstack/react-router'

import { PerformanceHistoryPage } from '@/features/performance-history/components/performance-history-page'
import { performanceHistorySearchSchema } from '@/features/performance-history/schema'
import { buildPerformanceHistorySearch } from '@/features/performance-history/search'

export const Route = createFileRoute(
  '/_authenticated/sites/$siteId/performance-history'
)({
  component: SitePerformanceHistoryRoute,
  validateSearch: performanceHistorySearchSchema,
})

function SitePerformanceHistoryRoute() {
  const { siteId } = Route.useParams()
  const search = buildPerformanceHistorySearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <PerformanceHistoryPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
      siteId={siteId}
    />
  )
}
