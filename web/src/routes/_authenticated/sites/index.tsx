import { createFileRoute } from '@tanstack/react-router'

import { SitesPage } from '@/features/sites/components/sites-page'
import { sitesSearchSchema } from '@/features/sites/schema'
import type { SiteSearch } from '@/features/sites/types'

export const Route = createFileRoute('/_authenticated/sites/')({
  component: SitesRoute,
  validateSearch: sitesSearchSchema,
})

function SitesRoute() {
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  const storedView = window.localStorage.getItem('sites:view-mode')
  const preferredView: SiteSearch['view'] =
    storedView === 'table' || storedView === 'card' ? storedView : 'card'
  const search: SiteSearch = {
    auth: rawSearch.auth,
    filter: rawSearch.filter ?? '',
    health: rawSearch.health,
    management: rawSearch.management,
    online: rawSearch.online,
    order: rawSearch.order ?? 'desc',
    page: rawSearch.page ?? 1,
    pageSize: rawSearch.pageSize ?? 20,
    sort: rawSearch.sort ?? 'priority',
    statistics: rawSearch.statistics,
    view: rawSearch.view ?? preferredView,
  }

  return (
    <SitesPage
      onOpenSite={(siteId, runId) =>
        void navigate({
          params: { siteId },
          search: runId == null ? undefined : { runId },
          to: '/sites/$siteId',
        })
      }
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
