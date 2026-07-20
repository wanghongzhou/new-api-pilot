import { createFileRoute } from '@tanstack/react-router'

import { LogsPage } from '@/features/logs/components/logs-page'
import { logSearchSchema } from '@/features/logs/schema'
import { buildLogSearch } from '@/features/logs/search'

export const Route = createFileRoute('/_authenticated/sites/$siteId/logs')({
  component: SiteLogsRoute,
  validateSearch: logSearchSchema,
})

function SiteLogsRoute() {
  const { siteId } = Route.useParams()
  const search = buildLogSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <LogsPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
      siteId={siteId}
    />
  )
}
