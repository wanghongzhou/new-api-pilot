import { createFileRoute } from '@tanstack/react-router'

import { LogsPage } from '@/features/logs/components/logs-page'
import { logSearchSchema } from '@/features/logs/schema'
import { buildLogSearch } from '@/features/logs/search'

export const Route = createFileRoute('/_authenticated/logs/')({
  component: GlobalLogsRoute,
  validateSearch: logSearchSchema,
})

function GlobalLogsRoute() {
  const search = buildLogSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <LogsPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
    />
  )
}
