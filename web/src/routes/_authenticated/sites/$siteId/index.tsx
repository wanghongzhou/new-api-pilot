import { createFileRoute } from '@tanstack/react-router'

import { SiteDetailPage } from '@/features/sites/components/site-detail-page'
import { siteDetailSearchSchema } from '@/features/sites/schema'

export const Route = createFileRoute('/_authenticated/sites/$siteId/')({
  component: SiteDetailRoute,
  validateSearch: siteDetailSearchSchema,
})

function SiteDetailRoute() {
  const { siteId } = Route.useParams()
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <SiteDetailPage
      onDeleted={() =>
        void navigate({
          search: {
            auth: [],
            health: [],
            management: [],
            online: [],
            statistics: [],
          },
          to: '/sites',
        })
      }
      onSearchChange={(changes) =>
        void navigate({
          search: (current) => ({ ...current, ...changes }),
        })
      }
      search={{
        runId: rawSearch.runId,
        runPage: rawSearch.runPage ?? 1,
        runStatus: rawSearch.runStatus,
        runTaskType: rawSearch.runTaskType,
        windowPage: rawSearch.windowPage ?? 1,
        windowStatus: rawSearch.windowStatus,
      }}
      siteId={siteId}
    />
  )
}
