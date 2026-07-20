import { createFileRoute } from '@tanstack/react-router'

import { ChannelInventoryPage } from '@/features/channel-inventory/components/channel-inventory-page'
import { channelInventorySearchSchema } from '@/features/channel-inventory/schema'
import { buildChannelInventorySearch } from '@/features/channel-inventory/search'

export const Route = createFileRoute('/_authenticated/channel-inventory/')({
  component: GlobalChannelInventoryRoute,
  validateSearch: channelInventorySearchSchema,
})

function GlobalChannelInventoryRoute() {
  const search = buildChannelInventorySearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <ChannelInventoryPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
    />
  )
}
