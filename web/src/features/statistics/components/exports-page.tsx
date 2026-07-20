import { Refresh01Icon, RepeatIcon, ViewIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Select } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
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

const exportStatuses: StatisticsExportStatus[] = [
  'pending',
  'running',
  'success',
  'failed',
  'expired',
]
const exportFormats: StatisticsExportFormat[] = ['xlsx', 'csv']
export const exportScopes: StatisticsExportScope[] = [
  'global',
  'site',
  'customer',
  'account',
  'model',
  'channel',
  'group',
  'token',
  'node',
  'logs',
  'user_inventory',
  'channel_inventory',
  'performance_history',
  'topup_inventory',
  'redemption_inventory',
  'upstream_tasks',
  'model_catalog',
  'model_rankings',
  'vendor_rankings',
  'subscription_plans',
  'pricing_catalog',
  'group_catalog',
  'system_tasks',
]
const exportSorts: StatisticsExportListSort[] = [
  'created_at',
  'finished_at',
  'status',
  'file_size',
]

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
    <article className='border-border bg-card grid gap-4 rounded-lg border p-4'>
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
          onClick={() => onOpen(job)}
          size='icon'
          title={t('exports.action.view')}
          variant='ghost'
        >
          <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
        </Button>
      </div>
      <ExportJobStatus job={job} />
      <dl className='grid grid-cols-2 gap-3 text-sm'>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.export.scope')}
          </dt>
          <dd>{exportScopeText(t, job.statistics_type)}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.export.format')}
          </dt>
          <dd>{exportFormatText(t, job.format)}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.export.task.rows')}
          </dt>
          <dd className='break-all'>{job.row_count}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.export.task.size')}
          </dt>
          <dd className='break-all'>{job.file_size}</dd>
        </div>
        <div className='col-span-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.export.task.createdAt')}
          </dt>
          <dd>
            <ExportTimestamp value={job.created_at} />
          </dd>
        </div>
      </dl>
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
    onError: (error) =>
      toast.error(t(dynamicI18nKey('api', getApiErrorTranslationKey(error)))),
    onSuccess: (job) => {
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
  const items = exportsQuery.data?.items ?? []
  const activeFilters = hasExportFilters(search)
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
    if (!first || !exportSorts.includes(first.id as StatisticsExportListSort)) {
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
        header: t('statistics.export.task.rows'),
      },
      {
        accessorKey: 'file_size',
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
      actions={
        <Button
          aria-label={t('common.refresh')}
          disabled={exportsQuery.isFetching}
          onClick={() => void exportsQuery.refetch()}
          size='icon'
          title={t('common.refresh')}
          variant='outline'
        >
          {exportsQuery.isFetching ? (
            <Spinner />
          ) : (
            <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
          )}
        </Button>
      }
      description={t('exports.description')}
      title={t('exports.title')}
    >
      <div className='grid min-w-0 gap-5'>
        <section
          aria-label={t('exports.filters.title')}
          className='border-border grid gap-4 border-y py-4 sm:grid-cols-3'
        >
          <fieldset className='grid gap-1.5 text-sm'>
            <legend className='font-medium'>{t('exports.table.status')}</legend>
            <div className='flex flex-wrap gap-x-4 gap-y-1'>
              {exportStatuses.map((status) => (
                <label
                  className='hover:bg-muted flex min-h-10 items-center gap-2 rounded-md px-2'
                  key={status}
                >
                  <input
                    checked={search.status.includes(status)}
                    className='accent-primary size-4'
                    onChange={() =>
                      onSearchChange({
                        page: 1,
                        status: search.status.includes(status)
                          ? search.status.filter((value) => value !== status)
                          : [...search.status, status],
                      })
                    }
                    type='checkbox'
                  />
                  {exportStatusText(t, status)}
                </label>
              ))}
            </div>
          </fieldset>
          <label className='grid gap-1.5 text-sm'>
            <span className='font-medium'>{t('statistics.export.format')}</span>
            <Select
              onChange={(event) =>
                onSearchChange({
                  format: event.target.value
                    ? (event.target.value as StatisticsExportFormat)
                    : undefined,
                  page: 1,
                })
              }
              value={search.format ?? ''}
            >
              <option value=''>{t('common.all')}</option>
              {exportFormats.map((format) => (
                <option key={format} value={format}>
                  {exportFormatText(t, format)}
                </option>
              ))}
            </Select>
          </label>
          <label className='grid gap-1.5 text-sm'>
            <span className='font-medium'>{t('statistics.export.scope')}</span>
            <Select
              onChange={(event) =>
                onSearchChange({
                  page: 1,
                  scope: event.target.value
                    ? (event.target.value as StatisticsExportScope)
                    : undefined,
                })
              }
              value={search.scope ?? ''}
            >
              <option value=''>{t('common.all')}</option>
              {exportScopes.map((scope) => (
                <option key={scope} value={scope}>
                  {exportScopeText(t, scope)}
                </option>
              ))}
            </Select>
          </label>
        </section>
        {activeFilters && (
          <Button className='w-fit' onClick={resetFilters} variant='ghost'>
            <HugeiconsIcon icon={RepeatIcon} strokeWidth={2} />
            {t('exports.filters.reset')}
          </Button>
        )}
        <DataTable
          ariaLabel={t('exports.table.label')}
          columns={columns}
          data={items}
          emptyAction={
            activeFilters ? (
              <Button onClick={resetFilters} variant='outline'>
                {t('exports.filters.reset')}
              </Button>
            ) : undefined
          }
          emptyDescription={t('exports.empty.description')}
          emptyTitle={t('exports.empty.title')}
          error={exportsQuery.isError}
          fetching={exportsQuery.isFetching}
          loading={exportsQuery.isPending}
          onPageChange={(page) => onSearchChange({ page })}
          onRetry={() => void exportsQuery.refetch()}
          onSortingChange={updateSorting}
          page={search.page}
          pageSize={search.pageSize}
          renderMobileCard={(job) => (
            <ExportJobCard job={job} onOpen={openJob} />
          )}
          sorting={[{ desc: search.order === 'desc', id: search.sort }]}
          total={exportsQuery.data?.total ?? 0}
        />
      </div>
      <ExportTaskSheet
        exportId={search.exportId}
        initialJob={selectedJob}
        onOpenChange={(open) => {
          if (!open) onSearchChange({ exportId: undefined })
        }}
        onRecreate={(job) => recreateMutation.mutate(job)}
        recreating={recreateMutation.isPending}
      />
    </SectionPageLayout>
  )
}
