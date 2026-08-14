import { createFileRoute } from '@tanstack/react-router'
import { useEffect } from 'react'

import { SitesPage } from '@/features/sites/components/sites-page'
import {
  siteSearchMiddlewares,
  sitesSearchSchema,
} from '@/features/sites/schema'
import type { SiteSearch } from '@/features/sites/types'

export const Route = createFileRoute('/_authenticated/sites/')({
  component: SitesRoute,
  search: { middlewares: siteSearchMiddlewares },
  validateSearch: sitesSearchSchema,
})

function SitesRoute() {
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  useEffect(() => {
    if (!window.location.search) return
    void navigate({ replace: true, search: (current) => current })
  }, [navigate])
  const storedView = window.localStorage.getItem('sites:view-mode-v2')
  const viewportDefault: SiteSearch['view'] = window.matchMedia(
    '(min-width: 1024px)'
  ).matches
    ? 'table'
    : 'card'
  const preferredView: SiteSearch['view'] =
    storedView === 'table' || storedView === 'card'
      ? storedView
      : viewportDefault
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
      onOpenSite={(siteId, runId) => {
        if (runId == null) {
          void navigate({ params: { siteId }, to: '/sites/$siteId' })
          return
        }
        window.location.assign(
          `/sites/${encodeURIComponent(siteId)}/collection-runs?runId=${encodeURIComponent(runId)}&tab=runs`
        )
      }}
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
