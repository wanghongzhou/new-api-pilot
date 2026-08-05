import { createFileRoute } from '@tanstack/react-router'

import { PricingGroupsPage } from '@/features/pricing-groups/components/pricing-groups-page'
import { pricingGroupSearchSchema } from '@/features/pricing-groups/schema'
import {
  buildPricingGroupSearch,
  serializePricingGroupSearch,
} from '@/features/pricing-groups/search'

export const Route = createFileRoute('/_authenticated/pricing-groups/')({
  component: GlobalPricingGroups,
  validateSearch: pricingGroupSearchSchema,
})

function GlobalPricingGroups() {
  const search = buildPricingGroupSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <PricingGroupsPage
      onPageReplace={(page) =>
        void navigate({
          replace: true,
          search: (current) =>
            serializePricingGroupSearch({
              ...buildPricingGroupSearch(current),
              page,
            }),
        })
      }
      onSearchChange={(changes) =>
        void navigate({
          search: (current) =>
            serializePricingGroupSearch({
              ...buildPricingGroupSearch(current),
              ...changes,
            }),
        })
      }
      search={search}
    />
  )
}
