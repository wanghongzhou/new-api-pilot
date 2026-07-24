import { createFileRoute } from '@tanstack/react-router'

import { PlatformUsersPage } from '@/features/platform-users/components/platform-users-page'
import { platformUserSearchSchema } from '@/features/platform-users/schema'

export const Route = createFileRoute('/_authenticated/settings/users')({
  component: PlatformUsersRoute,
  validateSearch: platformUserSearchSchema,
})

function PlatformUsersRoute() {
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  const search = {
    filter: rawSearch.filter ?? '',
    order: rawSearch.order,
    page: rawSearch.page ?? 1,
    pageSize: rawSearch.pageSize ?? 20,
    role: rawSearch.role,
    sort: rawSearch.sort,
    status: rawSearch.status,
  }

  return (
    <PlatformUsersPage
      onSearchChange={(changes) =>
        void navigate({
          replace: false,
          search: (current) => ({ ...current, ...changes }),
        })
      }
      search={search}
    />
  )
}
