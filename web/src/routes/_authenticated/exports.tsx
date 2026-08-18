import { createFileRoute } from '@tanstack/react-router'
import { useEffect, useMemo, useRef } from 'react'

import { ExportsPage } from '@/features/statistics/components/exports-page'
import { exportUrlSearch } from '@/features/statistics/exports-contract'
import { exportsSearchSchema } from '@/features/statistics/exports-schema'
import type { StatisticsExportSearch } from '@/features/statistics/types'
import { isIdString } from '@/lib/api-types'

export const Route = createFileRoute('/_authenticated/exports')({
  component: ExportsRoute,
  validateSearch: exportsSearchSchema,
})

function ExportsRoute() {
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  const normalizedInitialSearch = useRef(false)
  const search = useMemo<StatisticsExportSearch>(
    () => ({
      exportId: isIdString(rawSearch.exportId) ? rawSearch.exportId : undefined,
      format: rawSearch.format,
      order: rawSearch.order ?? 'desc',
      page: rawSearch.page ?? 1,
      pageSize: rawSearch.pageSize ?? 20,
      scope: rawSearch.scope,
      sort: rawSearch.sort ?? 'created_at',
      status: rawSearch.status ?? [],
    }),
    [
      rawSearch.exportId,
      rawSearch.format,
      rawSearch.order,
      rawSearch.page,
      rawSearch.pageSize,
      rawSearch.scope,
      rawSearch.sort,
      rawSearch.status,
    ]
  )
  useEffect(() => {
    if (normalizedInitialSearch.current || !window.location.search) return
    normalizedInitialSearch.current = true
    void navigate({ replace: true, search: exportUrlSearch(search) })
  }, [navigate, search])
  return (
    <ExportsPage
      onSearchChange={(changes) => {
        const owns = (key: keyof StatisticsExportSearch) =>
          Object.hasOwn(changes, key)
        const next: StatisticsExportSearch = {
          exportId: owns('exportId') ? changes.exportId : search.exportId,
          format: owns('format') ? changes.format : search.format,
          order: changes.order ?? search.order,
          page: changes.page ?? search.page,
          pageSize: changes.pageSize ?? search.pageSize,
          scope: owns('scope') ? changes.scope : search.scope,
          sort: changes.sort ?? search.sort,
          status: owns('status') ? (changes.status ?? []) : search.status,
        }
        void navigate({
          search: exportUrlSearch(next),
        })
      }}
      search={search}
    />
  )
}
