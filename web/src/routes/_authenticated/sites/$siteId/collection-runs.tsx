import { createFileRoute } from '@tanstack/react-router'
import { useEffect } from 'react'

import { SiteCollectionRunsPage } from '@/features/sites/components/site-collection-runs-page'
import { siteDetailSearchSchema } from '@/features/sites/schema'

export const Route = createFileRoute(
  '/_authenticated/sites/$siteId/collection-runs'
)({
  component: SiteCollectionRunsRoute,
  validateSearch: siteDetailSearchSchema,
})

function SiteCollectionRunsRoute() {
  const { siteId } = Route.useParams()
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  useEffect(() => {
    if (!rawSearch.runId || rawSearch.tab !== 'fast') return
    void navigate({
      replace: true,
      search: (current) => ({ ...current, tab: 'runs' }),
    })
  }, [navigate, rawSearch.runId, rawSearch.tab])
  return (
    <SiteCollectionRunsPage
      onSearchChange={(changes) =>
        void navigate({
          search: (current) => ({ ...current, ...changes }),
        })
      }
      search={{
        fastPage: rawSearch.fastPage ?? 1,
        fastStatus: rawSearch.fastStatus,
        fastTaskType: rawSearch.fastTaskType ?? 'site_probe',
        runId: rawSearch.runId,
        runPage: rawSearch.runPage ?? 1,
        runStatus: rawSearch.runStatus,
        runTaskType: rawSearch.runTaskType,
        tab: rawSearch.runId ? 'runs' : (rawSearch.tab ?? 'runs'),
        windowPage: rawSearch.windowPage ?? 1,
        windowStatus: rawSearch.windowStatus,
      }}
      siteId={siteId}
    />
  )
}
