import { createFileRoute } from '@tanstack/react-router'

import { ModelCatalogPage } from '@/features/model-catalog/components/model-catalog-page'
import { modelCatalogSearchSchema } from '@/features/model-catalog/schema'
import { buildModelCatalogSearch } from '@/features/model-catalog/search'

export const Route = createFileRoute(
  '/_authenticated/sites/$siteId/model-catalog'
)({
  component: SiteModelCatalogRoute,
  validateSearch: modelCatalogSearchSchema,
})

function SiteModelCatalogRoute() {
  const { siteId } = Route.useParams()
  const search = buildModelCatalogSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <ModelCatalogPage
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
      search={search}
      siteId={siteId}
    />
  )
}
