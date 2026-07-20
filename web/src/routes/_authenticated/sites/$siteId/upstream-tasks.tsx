import { createFileRoute } from '@tanstack/react-router'

import { UpstreamTasksPage } from '@/features/upstream-tasks/components/upstream-tasks-page'
import { upstreamTaskSearchSchema } from '@/features/upstream-tasks/schema'
import { buildUpstreamTaskSearch } from '@/features/upstream-tasks/search'

export const Route = createFileRoute(
  '/_authenticated/sites/$siteId/upstream-tasks'
)({
  component: SiteUpstreamTasksRoute,
  validateSearch: upstreamTaskSearchSchema,
})

function SiteUpstreamTasksRoute() {
  const { siteId } = Route.useParams()
  const search = buildUpstreamTaskSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <UpstreamTasksPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
      siteId={siteId}
    />
  )
}
