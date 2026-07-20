import { createFileRoute } from '@tanstack/react-router'

import { UserInventoryPage } from '@/features/user-inventory/components/user-inventory-page'
import { userInventorySearchSchema } from '@/features/user-inventory/schema'
import { buildUserInventorySearch } from '@/features/user-inventory/search'

export const Route = createFileRoute(
  '/_authenticated/sites/$siteId/user-inventory'
)({
  component: SiteUserInventoryRoute,
  validateSearch: userInventorySearchSchema,
})

function SiteUserInventoryRoute() {
  const { siteId } = Route.useParams()
  const search = buildUserInventorySearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <UserInventoryPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
      siteId={siteId}
    />
  )
}
