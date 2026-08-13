import { ViewIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import { useCallback, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { FacetedFilter } from '@/components/data/faceted-filter'
import { FilterPanel } from '@/components/data/filter-panel'
import { MetricValue } from '@/components/data/metric-value'
import { MultiFacetedFilter } from '@/components/data/multi-faceted-filter'
import { QueryStateAlert } from '@/components/data/query-state-alert'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { useRetainedQueryData } from '@/hooks/use-retained-query-data'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { translateMessageRef } from '@/lib/message-ref'

import { createStatisticsExport, listStatisticsExports } from '../api'
import { exportListParams, hasExportFilters } from '../exports-contract'
import { statisticsKeys } from '../query-keys'
import type {
  StatisticsExportFormat,
  StatisticsExportJobItem,
  StatisticsExportListSort,
  StatisticsExportScope,
  StatisticsExportSearch,
  StatisticsExportStatus,
} from '../types'
import { ExportTaskSheet } from './export-task-sheet'
import {
  exportFormatText,
  exportScopeText,
  exportStatusText,
  ExportStatusBadge,
  ExportTimestamp,
} from './export-ui'
import { exportScopeGroups } from './exports-filter-options'

const exportStatuses: StatisticsExportStatus[] = [
  'pending',
  'running',
  'success',
  'failed',
  'expired',
]
const exportFormats: StatisticsExportFormat[] = ['xlsx', 'csv']
const exportSorts: ReadonlySet<StatisticsExportListSort> = new Set([
  'created_at',
  'finished_at',
  'status',
  'file_size',
])

function ExportProgress({ job }: { job: StatisticsExportJobItem }) {
  const { t } = useTranslation()
  if (job.status !== 'pending' && job.status !== 'running') return null
  return (
    <div className='mt-2 grid gap-1'>
      <div className='text-muted-foreground flex justify-between text-xs'>
        <span>{t('statistics.export.task.progress')}</span>
        <span>{job.progress}%</span>
      </div>
      <progress
        aria-label={t('statistics.export.task.progress')}
        className='accent-primary h-2 w-full'
        max={100}
        value={job.progress}
      />
    </div>
  )
}

function ExportJobStatus({ job }: { job: StatisticsExportJobItem }) {
  return (
    <div className='min-w-36'>
      <ExportStatusBadge status={job.status} />
      <ExportProgress job={job} />
      {job.status === 'failed' && job.error && (
        <p className='text-destructive mt-2 max-w-64 text-xs break-words'>
          {translateMessageRef(job.error)}
        </p>
      )}
    </div>
  )
}

function ExportJobCard({
  job,
  onOpen,
}: {
  job: StatisticsExportJobItem
  onOpen: (job: StatisticsExportJobItem) => void
}) {
  const { t } = useTranslation()
  return (
    <article className='bg-card text-card-foreground ring-foreground/10 grid gap-4 rounded-xl p-4 ring-1'>
      <div className='flex min-w-0 items-start justify-between gap-2'>
        <div className='min-w-0'>
          <h2 className='font-semibold break-words'>
            {t('exports.card.title', { id: job.id })}
          </h2>
          <p className='text-muted-foreground mt-1 break-words'>
            {job.file_name || t('exports.value.notGenerated')}
          </p>
        </div>
        <Button
          aria-label={t('exports.action.view')}
          className='size-10'
          onClick={() => onOpen(job)}
          size='icon'
          title={t('exports.action.view')}
          variant='ghost'
        >
          <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
        </Button>
      </div>
      <ExportJobStatus job={job} />
      <div className='grid grid-cols-2 gap-3 text-sm'>
        <dl>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.export.scope')}
          </dt>
          <dd>{exportScopeText(t, job.statistics_type)}</dd>
        </dl>
        <dl>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.export.format')}
          </dt>
          <dd>{exportFormatText(t, job.format)}</dd>
        </dl>
        <dl>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.export.task.rows')}
          </dt>
          <dd className='break-all'>
            <MetricValue value={job.row_count} />
          </dd>
        </dl>
        <dl>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.export.task.size')}
          </dt>
          <dd className='break-all'>
            <MetricValue value={job.file_size} />
          </dd>
        </dl>
        <dl className='col-span-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.export.task.createdAt')}
          </dt>
          <dd>
            <ExportTimestamp value={job.created_at} />
          </dd>
        </dl>
      </div>
      <Button onClick={() => onOpen(job)} variant='outline'>
        <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
        {t('exports.action.view')}
      </Button>
    </article>
  )
}

export function ExportsPage({
  onSearchChange,
  search,
}: {
  onSearchChange: (changes: Partial<StatisticsExportSearch>) => void
  search: StatisticsExportSearch
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedJob, setSelectedJob] = useState<StatisticsExportJobItem>()
  const recreatingRef = useRef(false)
  const params = useMemo(() => exportListParams(search), [search])
  const exportsQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => listStatisticsExports(params),
    queryKey: statisticsKeys.exportList(params),
    refetchInterval: (query) =>
      query.state.data?.items.some(
        (item) => item.status === 'pending' || item.status === 'running'
      )
        ? 2_000
        : false,
    staleTime: 2_000,
  })
  const recreateMutation = useMutation({
    mutationFn: (job: StatisticsExportJobItem) =>
      createStatisticsExport({
        filters: job.filters,
        format: job.format,
        statistics_type: job.statistics_type,
      }),
    onError: (error) => {
      recreatingRef.current = false
      toast.error(t(dynamicI18nKey('api', getApiErrorTranslationKey(error))))
    },
    onSuccess: (job) => {
      recreatingRef.current = false
      toast.success(
        job.deduplicated
          ? t('statistics.export.toast.deduplicated')
          : t('statistics.export.toast.created')
      )
      queryClient.setQueryData(statisticsKeys.export(job.id), job)
      void queryClient.invalidateQueries({
        queryKey: statisticsKeys.exportLists(),
      })
      setSelectedJob(job)
      onSearchChange({ exportId: job.id })
    },
  })
  const data = useRetainedQueryData(exportsQuery.data, exportsQuery.isError)
  const items = data?.items ?? []
  const recreate = (job: StatisticsExportJobItem) => {
    if (recreatingRef.current) return
    recreatingRef.current = true
    recreateMutation.mutate(job)
  }
  const activeFilters = hasExportFilters(search)
  const statusOptions = useMemo(
    () =>
      exportStatuses.map((status) => ({
        label: exportStatusText(t, status),
        value: status,
      })),
    [t]
  )
  const formatOptions = useMemo(
    () =>
      exportFormats.map((format) => ({
        label: exportFormatText(t, format),
        value: format,
      })),
    [t]
  )
  const scopeOptions = useMemo(() => {
    const groupLabels = {
      finance: t('exports.filters.group.finance'),
      operations: t('exports.filters.group.operations'),
      resources: t('exports.filters.group.resources'),
      tasks: t('exports.filters.group.tasks'),
    }
    return exportScopeGroups.flatMap((group) =>
      group.scopes.map((scope) => ({
        group: groupLabels[group.key],
        label: exportScopeText(t, scope),
        value: scope,
      }))
    )
  }, [t])
  const openJob = useCallback(
    (job: StatisticsExportJobItem) => {
      setSelectedJob(job)
      onSearchChange({ exportId: job.id })
    },
    [onSearchChange]
  )
  const updateSorting = (
    updater: SortingState | ((old: SortingState) => SortingState)
  ) => {
    const current = [{ desc: search.order === 'desc', id: search.sort }]
    const next = typeof updater === 'function' ? updater(current) : updater
    const first = next[0]
    if (!first || !exportSorts.has(first.id as StatisticsExportListSort)) {
      return
    }
    onSearchChange({
      order: first.desc ? 'desc' : 'asc',
      page: 1,
      sort: first.id as StatisticsExportListSort,
    })
  }
  const columns = useMemo<ColumnDef<StatisticsExportJobItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <button
            className='font-medium hover:underline'
            onClick={() => openJob(row.original)}
            type='button'
          >
            {row.original.id}
          </button>
        ),
        header: t('statistics.export.task.id'),
        id: 'id',
      },
      {
        cell: ({ row }) => <ExportJobStatus job={row.original} />,
        enableSorting: true,
        header: t('exports.table.status'),
        id: 'status',
      },
      {
        cell: ({ row }) => exportScopeText(t, row.original.statistics_type),
        header: t('statistics.export.scope'),
        id: 'scope',
      },
      {
        cell: ({ row }) => exportFormatText(t, row.original.format),
        header: t('statistics.export.format'),
        id: 'format',
      },
      {
        cell: ({ row }) => (
          <span className='max-w-64 break-words'>
            {row.original.file_name || t('exports.value.notGenerated')}
          </span>
        ),
        header: t('exports.table.file'),
        id: 'file',
      },
      {
        accessorKey: 'row_count',
        cell: ({ row }) => <MetricValue value={row.original.row_count} />,
        header: t('statistics.export.task.rows'),
      },
      {
        accessorKey: 'file_size',
        cell: ({ row }) => <MetricValue value={row.original.file_size} />,
        enableSorting: true,
        header: t('statistics.export.task.size'),
        id: 'file_size',
      },
      {
        cell: ({ row }) => <ExportTimestamp value={row.original.created_at} />,
        enableSorting: true,
        header: t('statistics.export.task.createdAt'),
        id: 'created_at',
        sortDescFirst: true,
      },
      {
        cell: ({ row }) => <ExportTimestamp value={row.original.finished_at} />,
        enableSorting: true,
        header: t('exports.table.finishedAt'),
        id: 'finished_at',
        sortDescFirst: true,
      },
      {
        cell: ({ row }) => (
          <Button
            onClick={() => openJob(row.original)}
            size='sm'
            variant='outline'
          >
            <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
            {t('exports.action.view')}
          </Button>
        ),
        header: t('common.actions'),
        id: 'actions',
      },
    ],
    [openJob, t]
  )
  const resetFilters = () =>
    onSearchChange({
      format: undefined,
      page: 1,
      scope: undefined,
      status: [],
    })
  return (
    <SectionPageLayout
      fixedContent
      description={t('exports.description')}
      title={t('exports.title')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        <FilterPanel
          description={t('exports.description')}
          hasActiveFilters={activeFilters}
          onReset={activeFilters ? resetFilters : undefined}
          title={t('exports.filters.title')}
        >
          <MultiFacetedFilter
            className='w-full justify-start sm:w-auto'
            clearLabel={t('common.clearFilters')}
            onChange={(values) =>
              onSearchChange({
                page: 1,
                status: values as StatisticsExportStatus[],
              })
            }
            options={statusOptions}
            title={t('exports.table.status')}
            values={search.status}
          />
          <FacetedFilter
            className='w-full justify-between sm:w-40'
            clearLabel={t('common.clearFilters')}
            onChange={(value) =>
              onSearchChange({
                format: value ? (value as StatisticsExportFormat) : undefined,
                page: 1,
              })
            }
            options={formatOptions}
            title={t('statistics.export.format')}
            value={search.format ?? ''}
          />
          <FacetedFilter
            className='w-full justify-between sm:w-64'
            clearLabel={t('common.clearFilters')}
            onChange={(value) =>
              onSearchChange({
                page: 1,
                scope: value ? (value as StatisticsExportScope) : undefined,
              })
            }
            options={scopeOptions}
            title={t('statistics.export.scope')}
            value={search.scope ?? ''}
          />
        </FilterPanel>
        {exportsQuery.isError && data && (
          <QueryStateAlert
            message={t('common.retainedDataRefreshFailed')}
            onRetry={() => void exportsQuery.refetch()}
          />
        )}
        <DataTable
          ariaLabel={t('exports.table.label')}
          columns={columns}
          data={items}
          emptyDescription={t('exports.empty.description')}
          emptyTitle={t('exports.empty.title')}
          error={exportsQuery.isError}
          fetching={exportsQuery.isFetching}
          loading={exportsQuery.isPending}
          onPageChange={(page) => onSearchChange({ page })}
          onPageSizeChange={(pageSize) => onSearchChange({ page: 1, pageSize })}
          onRetry={() => void exportsQuery.refetch()}
          onSortingChange={updateSorting}
          page={search.page}
          pageSize={search.pageSize}
          renderMobileCard={(job) => (
            <ExportJobCard job={job} onOpen={openJob} />
          )}
          sorting={[{ desc: search.order === 'desc', id: search.sort }]}
          total={data?.total ?? 0}
        />
      </div>
      <ExportTaskSheet
        exportId={search.exportId}
        initialJob={selectedJob}
        onOpenChange={(open) => {
          if (!open) onSearchChange({ exportId: undefined })
        }}
        onRecreate={recreate}
        recreating={recreateMutation.isPending}
      />
    </SectionPageLayout>
  )
}
