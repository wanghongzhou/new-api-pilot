import { createFileRoute } from '@tanstack/react-router'

import { PerformanceHistoryPage } from '@/features/performance-history/components/performance-history-page'
import { performanceHistorySearchSchema } from '@/features/performance-history/schema'
import { buildPerformanceHistorySearch } from '@/features/performance-history/search'

export const Route = createFileRoute('/_authenticated/performance-history/')({
  component: GlobalPerformanceHistoryRoute,
  validateSearch: performanceHistorySearchSchema,
})

function GlobalPerformanceHistoryRoute() {
  const search = buildPerformanceHistorySearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <PerformanceHistoryPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
    />
  )
}
