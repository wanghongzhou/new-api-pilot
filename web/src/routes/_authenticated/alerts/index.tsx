import { createFileRoute } from '@tanstack/react-router'
import { useEffect } from 'react'

import { AlertsPage } from '@/features/alerts/components/alerts-page'
import {
  alertSearchMiddlewares,
  alertsSearchSchema,
} from '@/features/alerts/schema'
import type { AlertSearch } from '@/features/alerts/types'
import { isIdString } from '@/lib/api-types'

export const Route = createFileRoute('/_authenticated/alerts/')({
  component: AlertsRoute,
  search: { middlewares: alertSearchMiddlewares },
  validateSearch: alertsSearchSchema,
})

function AlertsRoute() {
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  useEffect(() => {
    if (!window.location.search) return
    void navigate({ replace: true, search: (current) => current })
  }, [navigate])
  const search: AlertSearch = {
    alertId: isIdString(rawSearch.alertId) ? rawSearch.alertId : undefined,
    end: rawSearch.end,
    level: rawSearch.level,
    order: rawSearch.order ?? 'desc',
    page: rawSearch.page ?? 1,
    pageSize: rawSearch.pageSize ?? 20,
    ruleCategory: rawSearch.ruleCategory,
    ruleEnabled: rawSearch.ruleEnabled,
    ruleInherited: rawSearch.ruleInherited,
    ruleLevel: rawSearch.ruleLevel,
    ruleOrder: rawSearch.ruleOrder ?? 'asc',
    rulePage: rawSearch.rulePage ?? 1,
    rulePageSize: rawSearch.rulePageSize ?? 20,
    ruleSiteId: isIdString(rawSearch.ruleSiteId)
      ? rawSearch.ruleSiteId
      : undefined,
    ruleSort: rawSearch.ruleSort,
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
            ruleCategory: owns('ruleCategory')
              ? changes.ruleCategory
              : current.ruleCategory,
            ruleEnabled: owns('ruleEnabled')
              ? changes.ruleEnabled
              : current.ruleEnabled,
            ruleInherited: owns('ruleInherited')
              ? changes.ruleInherited
              : current.ruleInherited,
            ruleLevel: owns('ruleLevel')
              ? changes.ruleLevel
              : current.ruleLevel,
            ruleOrder: changes.ruleOrder ?? current.ruleOrder,
            rulePage: changes.rulePage ?? current.rulePage,
            rulePageSize: changes.rulePageSize ?? current.rulePageSize,
            ruleSiteId: owns('ruleSiteId')
              ? changes.ruleSiteId
              : current.ruleSiteId,
            ruleSort: owns('ruleSort') ? changes.ruleSort : current.ruleSort,
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
