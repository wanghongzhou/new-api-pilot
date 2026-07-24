import { createFileRoute } from '@tanstack/react-router'

import { ModelCatalogPage } from '@/features/model-catalog/components/model-catalog-page'
import { modelCatalogSearchSchema } from '@/features/model-catalog/schema'
import {
  buildModelCatalogSearch,
  serializeModelCatalogSearch,
} from '@/features/model-catalog/search'

export const Route = createFileRoute('/_authenticated/model-catalog/')({
  component: GlobalModelCatalogRoute,
  validateSearch: modelCatalogSearchSchema,
})

function GlobalModelCatalogRoute() {
  const search = buildModelCatalogSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <ModelCatalogPage
      onSearchChange={(changes) =>
        void navigate({
          search: (current) =>
            serializeModelCatalogSearch({
              ...buildModelCatalogSearch(current),
              ...changes,
            }),
        })
      }
      search={search}
    />
  )
}
