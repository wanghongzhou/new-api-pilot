import { createFileRoute } from '@tanstack/react-router'

import { FinancialOperationsPage } from '@/features/financial-operations/components/financial-operations-page'
import { financialOperationsSearchSchema } from '@/features/financial-operations/schema'
import { buildFinancialOperationsSearch } from '@/features/financial-operations/search'

export const Route = createFileRoute(
  '/_authenticated/sites/$siteId/financial-operations'
)({
  component: SiteFinancialOperationsRoute,
  validateSearch: financialOperationsSearchSchema,
})

function SiteFinancialOperationsRoute() {
  const { siteId } = Route.useParams()
  const search = buildFinancialOperationsSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <FinancialOperationsPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
      siteId={siteId}
    />
  )
}
