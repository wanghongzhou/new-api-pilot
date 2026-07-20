import { createFileRoute } from '@tanstack/react-router'

import { UpstreamTasksPage } from '@/features/upstream-tasks/components/upstream-tasks-page'
import { upstreamTaskSearchSchema } from '@/features/upstream-tasks/schema'
import { buildUpstreamTaskSearch } from '@/features/upstream-tasks/search'

export const Route = createFileRoute('/_authenticated/upstream-tasks/')({
  component: GlobalUpstreamTasksRoute,
  validateSearch: upstreamTaskSearchSchema,
})

function GlobalUpstreamTasksRoute() {
  const search = buildUpstreamTaskSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <UpstreamTasksPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
    />
  )
}
