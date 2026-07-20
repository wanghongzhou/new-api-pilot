import { createFileRoute } from '@tanstack/react-router'

import { AlertsPage } from '@/features/alerts/components/alerts-page'
import { alertsSearchSchema } from '@/features/alerts/schema'
import type { AlertSearch } from '@/features/alerts/types'
import { isIdString } from '@/lib/api-types'

export const Route = createFileRoute('/_authenticated/alerts/')({
  component: AlertsRoute,
  validateSearch: alertsSearchSchema,
})

function AlertsRoute() {
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  const search: AlertSearch = {
    alertId: isIdString(rawSearch.alertId) ? rawSearch.alertId : undefined,
    end: rawSearch.end,
    level: rawSearch.level,
    order: rawSearch.order ?? 'desc',
    page: rawSearch.page ?? 1,
    pageSize: rawSearch.pageSize ?? 20,
    ruleSiteId: isIdString(rawSearch.ruleSiteId)
      ? rawSearch.ruleSiteId
      : undefined,
    scope: rawSearch.scope ?? 'global',
    siteId: isIdString(rawSearch.siteId) ? rawSearch.siteId : undefined,
    sort: rawSearch.sort,
    start: rawSearch.start,
    status: rawSearch.status,
    tab: rawSearch.tab ?? 'events',
    targetType: rawSearch.targetType,
  }
  return (
    <AlertsPage
      onSearchChange={(changes) => {
        const owns = (key: keyof AlertSearch) => Object.hasOwn(changes, key)
        void navigate({
          search: (current) => ({
            ...current,
            alertId: owns('alertId') ? changes.alertId : current.alertId,
            end: owns('end') ? changes.end : current.end,
            level: owns('level') ? changes.level : current.level,
            order: changes.order ?? current.order,
            page: changes.page ?? current.page,
            pageSize: changes.pageSize ?? current.pageSize,
            ruleSiteId: owns('ruleSiteId')
              ? changes.ruleSiteId
              : current.ruleSiteId,
            scope: changes.scope ?? current.scope,
            siteId: owns('siteId') ? changes.siteId : current.siteId,
            sort: owns('sort') ? changes.sort : current.sort,
            start: owns('start') ? changes.start : current.start,
            status: owns('status') ? changes.status : current.status,
            tab: changes.tab ?? current.tab,
            targetType: owns('targetType')
              ? changes.targetType
              : current.targetType,
          }),
        })
      }}
      search={search}
    />
  )
}
