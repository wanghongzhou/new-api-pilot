import { createFileRoute } from '@tanstack/react-router'

import { SubscriptionPlansPage } from '@/features/subscription-plans/components/subscription-plans-page'
import { subscriptionPlanSearchSchema } from '@/features/subscription-plans/schema'
import { buildSubscriptionPlanSearch } from '@/features/subscription-plans/search'

export const Route = createFileRoute('/_authenticated/subscription-plans/')({
  component: GlobalSubscriptionPlans,
  validateSearch: subscriptionPlanSearchSchema,
})

function GlobalSubscriptionPlans() {
  const search = buildSubscriptionPlanSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <SubscriptionPlansPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
    />
  )
}
