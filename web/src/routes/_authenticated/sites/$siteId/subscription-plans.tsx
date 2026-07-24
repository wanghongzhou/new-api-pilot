import { createFileRoute } from '@tanstack/react-router'

import { SubscriptionPlansPage } from '@/features/subscription-plans/components/subscription-plans-page'
import { subscriptionPlanSearchSchema } from '@/features/subscription-plans/schema'
import {
  buildSubscriptionPlanSearch,
  serializeSubscriptionPlanSearch,
} from '@/features/subscription-plans/search'

export const Route = createFileRoute(
  '/_authenticated/sites/$siteId/subscription-plans'
)({
  component: SiteSubscriptionPlans,
  validateSearch: subscriptionPlanSearchSchema,
})

function SiteSubscriptionPlans() {
  const { siteId } = Route.useParams()
  const search = buildSubscriptionPlanSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <SubscriptionPlansPage
      onSearchChange={(changes) =>
        void navigate({
          search: (current) =>
            serializeSubscriptionPlanSearch({
              ...buildSubscriptionPlanSearch(current),
              ...changes,
            }),
        })
      }
      search={search}
      siteId={siteId}
    />
  )
}
