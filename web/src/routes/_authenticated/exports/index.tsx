import { createFileRoute } from '@tanstack/react-router'

import { ExportsPage } from '@/features/statistics/components/exports-page'
import { exportsSearchSchema } from '@/features/statistics/exports-schema'
import type { StatisticsExportSearch } from '@/features/statistics/types'
import { isIdString } from '@/lib/api-types'

export const Route = createFileRoute('/_authenticated/exports/')({
  component: ExportsRoute,
  validateSearch: exportsSearchSchema,
})

function ExportsRoute() {
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  const search: StatisticsExportSearch = {
    exportId: isIdString(rawSearch.exportId) ? rawSearch.exportId : undefined,
    format: rawSearch.format,
    order: rawSearch.order ?? 'desc',
    page: rawSearch.page ?? 1,
    pageSize: rawSearch.pageSize ?? 20,
    scope: rawSearch.scope,
    sort: rawSearch.sort ?? 'created_at',
    status: rawSearch.status ?? [],
  }
  return (
    <ExportsPage
      onSearchChange={(changes) => {
        const owns = (key: keyof StatisticsExportSearch) =>
          Object.hasOwn(changes, key)
        void navigate({
          search: (current) => ({
            ...current,
            exportId: owns('exportId') ? changes.exportId : current.exportId,
            format: owns('format') ? changes.format : current.format,
            order: changes.order ?? current.order,
            page: changes.page ?? current.page,
            pageSize: changes.pageSize ?? current.pageSize,
            scope: owns('scope') ? changes.scope : current.scope,
            sort: changes.sort ?? current.sort,
            status: owns('status') ? changes.status : current.status,
          }),
        })
      }}
      search={search}
    />
  )
}
