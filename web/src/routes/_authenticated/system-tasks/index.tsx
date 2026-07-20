import { createFileRoute } from '@tanstack/react-router'

import { SystemTasksPage } from '@/features/system-tasks/components/system-tasks-page'
import { systemTaskSearchSchema } from '@/features/system-tasks/schema'
import { buildSystemTaskSearch } from '@/features/system-tasks/search'

export const Route = createFileRoute('/_authenticated/system-tasks/')({
  component: GlobalSystemTasks,
  validateSearch: systemTaskSearchSchema,
})

function GlobalSystemTasks() {
  const search = buildSystemTaskSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <SystemTasksPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
    />
  )
}
