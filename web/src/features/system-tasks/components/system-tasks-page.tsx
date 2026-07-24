import { ArrowLeft01Icon, FileExportIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataStatusBadge } from '@/components/data/data-status'
import { FilterPanel } from '@/components/data/filter-panel'
import { MetricValue } from '@/components/data/metric-value'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Input } from '@/components/ui/input'
import { createStatisticsExport } from '@/features/statistics/api'
import { ExportTaskSheet } from '@/features/statistics/components/export-task-sheet'
import type {
  StatisticsExportFormat,
  StatisticsExportJobItem,
} from '@/features/statistics/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { isIdString, parseIdString } from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'
import { formatNumericDisplayValue } from '@/lib/display-value'
import { hasFilterChanges } from '@/lib/filter-state'

import {
  getSiteSystemTaskStatistics,
  getSystemTaskStatistics,
  listSiteSystemTasks,
  listSystemTasks,
} from '../api'
import { buildSystemTaskExportRequest } from '../export-request'
import { systemTaskKeys } from '../query-keys'
import { buildSystemTaskSearch, type SystemTaskSearch } from '../search'
import {
  systemTaskStatuses,
  systemTaskTypes,
  type SystemTaskBreakdown,
  type SystemTaskItem,
  type SystemTaskMetric,
  type SystemTaskQueryParams,
  type SystemTaskStatus,
  type SystemTaskType,
} from '../types'

function params(search: SystemTaskSearch): SystemTaskQueryParams {
  return {
    created_end: search.createdEnd,
    created_start: search.createdStart,
    error_present: search.errorPresent,
    p: search.page,
    page_size: search.pageSize,
    site_ids: search.siteIds,
    statuses: search.statuses,
    types: search.types,
  }
}

function timestamp(value: number | null) {
  return value == null || value <= 0
    ? '-'
    : fromUnixSeconds(value).format('YYYY-MM-DD HH:mm:ss')
}
function dateTimeValue(value?: number) {
  return value == null ? '' : fromUnixSeconds(value).format('YYYY-MM-DDTHH:mm')
}
function parseDateTime(value: string) {
  if (!value) return undefined
  const parsed = dayjs.tz(value, 'YYYY-MM-DDTHH:mm', BEIJING_TIMEZONE)
  return parsed.isValid() ? parsed.unix() : undefined
}

function taskTypeText(t: (key: string) => string, type: SystemTaskType) {
  if (type === 'log_cleanup') return t('systemTasks.type.log_cleanup')
  if (type === 'channel_test') return t('systemTasks.type.channel_test')
  if (type === 'model_update') return t('systemTasks.type.model_update')
  if (type === 'midjourney_poll') return t('systemTasks.type.midjourney_poll')
  return t('systemTasks.type.async_task_poll')
}
function taskStatusText(t: (key: string) => string, status: SystemTaskStatus) {
  if (status === 'pending') return t('systemTasks.status.pending')
  if (status === 'running') return t('systemTasks.status.running')
  if (status === 'succeeded') return t('systemTasks.status.succeeded')
  return t('systemTasks.status.failed')
}
function errorCodeText(
  t: (key: string) => string,
  code: SystemTaskItem['error_code']
) {
  if (code === 'UPSTREAM_SYSTEM_TASK_FAILED') {
    return t('systemTasks.errorCode.UPSTREAM_SYSTEM_TASK_FAILED')
  }
  if (code === 'UPSTREAM_SYSTEM_TASK_LEASE_EXPIRED') {
    return t('systemTasks.errorCode.UPSTREAM_SYSTEM_TASK_LEASE_EXPIRED')
  }
  if (code === 'UPSTREAM_SYSTEM_TASK_INVALID_RESPONSE') {
    return t('systemTasks.errorCode.UPSTREAM_SYSTEM_TASK_INVALID_RESPONSE')
  }
  return t('systemTasks.errorCode.unknown')
}
function StatusBadge({ status }: { status: SystemTaskStatus }) {
  const { t } = useTranslation()
  let variant: 'destructive' | 'primary' | 'success' | 'warning' = 'warning'
  if (status === 'succeeded') variant = 'success'
  else if (status === 'failed') variant = 'destructive'
  else if (status === 'running') variant = 'primary'
  return <Badge variant={variant}>{taskStatusText(t, status)}</Badge>
}

function errorFilterText(
  t: (key: string) => string,
  value: boolean | undefined
) {
  if (value == null) return t('common.all')
  return value ? t('systemTasks.error.yes') : t('systemTasks.error.no')
}

function MetricGrid({ metric }: { metric: SystemTaskMetric }) {
  const { t } = useTranslation()
  const values = [
    [t('systemTasks.metric.total'), metric.total],
    [t('systemTasks.metric.active'), metric.active],
    [t('systemTasks.metric.succeeded'), metric.succeeded],
    [t('systemTasks.metric.failed'), metric.failed],
    [t('systemTasks.metric.errorPresent'), metric.error_present],
  ] as const
  return (
    <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-5'>
      {values.map(([label, value]) => (
        <div className='border-border p-3 sm:border-r' key={label}>
          <dt className='text-muted-foreground text-xs'>{label}</dt>
          <dd className='mt-1 text-lg font-semibold'>
            <MetricValue value={value} />
          </dd>
        </div>
      ))}
    </dl>
  )
}

function Breakdown({
  items,
  title,
}: {
  items: SystemTaskBreakdown[]
  title: string
}) {
  return (
    <section className='grid content-start gap-2'>
      <h2 className='font-semibold'>{title}</h2>
      {items.map((item) => (
        <article
          className='border-border grid gap-2 rounded-lg border p-3'
          key={`${item.dimension_id}:${item.site_id}`}
        >
          <div className='flex items-start justify-between gap-2'>
            <div>
              <p className='font-medium'>
                {item.dimension_name || item.site_name}
              </p>
              <code className='text-muted-foreground text-xs'>
                {item.dimension_id || item.site_id}
              </code>
            </div>
            <DataStatusBadge status={item.data_status} />
          </div>
          <MetricGrid metric={item} />
          <span className='text-muted-foreground text-xs'>
            {timestamp(item.as_of)}
          </span>
        </article>
      ))}
    </section>
  )
}

function Filters({
  global,
  onChange,
  search,
}: {
  global: boolean
  onChange: (changes: Partial<SystemTaskSearch>) => void
  search: SystemTaskSearch
}) {
  const { t } = useTranslation()
  const toggle = <T extends string>(
    values: T[],
    value: T,
    key: 'statuses' | 'types'
  ) =>
    onChange({
      [key]: values.includes(value)
        ? values.filter((item) => item !== value)
        : [...values, value],
      page: 1,
    })
  const reset = buildSystemTaskSearch({ pageSize: search.pageSize })
  return (
    <FilterPanel
      description={t('systemTasks.filters.description')}
      hasActiveFilters={hasFilterChanges(search, reset, [
        'createdEnd',
        'createdStart',
        'errorPresent',
        'siteIds',
        'statuses',
        'types',
      ])}
      onReset={() => onChange(reset)}
      title={t('systemTasks.filters.title')}
    >
      <div className='grid min-w-0 flex-1 gap-3 md:grid-cols-3'>
        {global && (
          <label className='grid gap-1 text-sm'>
            <span>{t('systemTasks.filters.siteIds')}</span>
            <Input
              inputMode='numeric'
              value={search.siteIds.join(',')}
              onChange={(event) =>
                onChange({
                  page: 1,
                  siteIds: event.target.value
                    .split(',')
                    .map((v) => v.trim())
                    .filter(isIdString)
                    .map(parseIdString),
                })
              }
            />
          </label>
        )}
        <label className='grid gap-1 text-sm'>
          <span>{t('systemTasks.filters.createdStart')}</span>
          <Input
            type='datetime-local'
            value={dateTimeValue(search.createdStart)}
            onChange={(event) =>
              onChange({
                createdStart: parseDateTime(event.target.value),
                page: 1,
              })
            }
          />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('systemTasks.filters.createdEnd')}</span>
          <Input
            type='datetime-local'
            value={dateTimeValue(search.createdEnd)}
            onChange={(event) =>
              onChange({
                createdEnd: parseDateTime(event.target.value),
                page: 1,
              })
            }
          />
        </label>
      </div>
      <fieldset className='grid gap-2'>
        <legend className='text-sm'>{t('systemTasks.filters.types')}</legend>
        <div className='flex flex-wrap gap-2'>
          {systemTaskTypes.map((type) => (
            <Button
              aria-pressed={search.types.includes(type)}
              key={type}
              onClick={() => toggle(search.types, type, 'types')}
              size='sm'
              type='button'
              variant={search.types.includes(type) ? 'secondary' : 'outline'}
            >
              {taskTypeText(t, type)}
            </Button>
          ))}
        </div>
      </fieldset>
      <fieldset className='grid gap-2'>
        <legend className='text-sm'>{t('systemTasks.filters.statuses')}</legend>
        <div className='flex flex-wrap gap-2'>
          {systemTaskStatuses.map((status) => (
            <Button
              aria-pressed={search.statuses.includes(status)}
              key={status}
              onClick={() => toggle(search.statuses, status, 'statuses')}
              size='sm'
              type='button'
              variant={
                search.statuses.includes(status) ? 'secondary' : 'outline'
              }
            >
              {taskStatusText(t, status)}
            </Button>
          ))}
        </div>
      </fieldset>
      <fieldset className='grid gap-2'>
        <legend className='text-sm'>
          {t('systemTasks.filters.errorPresent')}
        </legend>
        <div className='flex flex-wrap gap-2'>
          {([undefined, true, false] as const).map((value) => (
            <Button
              aria-pressed={search.errorPresent === value}
              key={String(value)}
              onClick={() => onChange({ errorPresent: value, page: 1 })}
              size='sm'
              type='button'
              variant={search.errorPresent === value ? 'secondary' : 'outline'}
            >
              {errorFilterText(t, value)}
            </Button>
          ))}
        </div>
      </fieldset>
    </FilterPanel>
  )
}

function ProgressView({ item }: { item: SystemTaskItem }) {
  const { t } = useTranslation()
  if (!item.progress) return <span className='text-muted-foreground'>-</span>
  return (
    <dl className='grid min-w-40 gap-1 text-xs'>
      <div>
        <dt className='inline'>{t('systemTasks.progress.percent')}：</dt>
        <dd className='inline'>
          {formatNumericDisplayValue(item.progress.progress)}
        </dd>
      </div>
      <div>
        <dt className='inline'>{t('systemTasks.progress.processed')}：</dt>
        <dd className='inline'>
          {formatNumericDisplayValue(item.progress.processed)} /{' '}
          {formatNumericDisplayValue(item.progress.total)}
        </dd>
      </div>
      <div>
        <dt className='inline'>{t('systemTasks.progress.remaining')}：</dt>
        <dd className='inline'>
          {formatNumericDisplayValue(item.progress.remaining)}
        </dd>
      </div>
    </dl>
  )
}

function ResultView({ item }: { item: SystemTaskItem }) {
  const { t } = useTranslation()
  if (!item.result) return <span className='text-muted-foreground'>-</span>
  const entries: [string, string | null][] = []
  if (item.type === 'log_cleanup') {
    entries.push([
      t('systemTasks.result.deletedCount'),
      item.result.deleted_count,
    ])
  }
  if (item.type === 'channel_test') {
    entries.push(
      [t('systemTasks.result.tested'), item.result.tested],
      [t('systemTasks.metric.succeeded'), item.result.succeeded],
      [t('systemTasks.metric.failed'), item.result.failed],
      [t('systemTasks.result.disabled'), item.result.disabled],
      [t('systemTasks.result.enabled'), item.result.enabled]
    )
  }
  if (item.type === 'model_update') {
    entries.push(
      [t('systemTasks.result.checkedChannels'), item.result.checked_channels],
      [t('systemTasks.result.changedChannels'), item.result.changed_channels],
      [
        t('systemTasks.result.detectedAddModels'),
        item.result.detected_add_models,
      ],
      [
        t('systemTasks.result.detectedRemoveModels'),
        item.result.detected_remove_models,
      ],
      [t('systemTasks.result.failedChannels'), item.result.failed_channels],
      [t('systemTasks.result.autoAddedModels'), item.result.auto_added_models]
    )
  }
  if (item.type === 'midjourney_poll') {
    entries.push(
      [t('systemTasks.result.unfinishedTasks'), item.result.unfinished_tasks],
      [t('systemTasks.result.channelsScanned'), item.result.channels_scanned],
      [t('systemTasks.result.nullTasksFailed'), item.result.null_tasks_failed]
    )
  }
  if (item.type === 'async_task_poll') {
    entries.push(
      [t('systemTasks.result.unfinishedTasks'), item.result.unfinished_tasks],
      [t('systemTasks.result.platformsScanned'), item.result.platforms_scanned],
      [t('systemTasks.result.nullTasksFailed'), item.result.null_tasks_failed]
    )
  }
  return (
    <dl className='grid min-w-48 gap-1 text-xs'>
      {entries.map(([label, value]) => (
        <div key={label}>
          <dt className='inline'>{label}：</dt>
          <dd className='inline'>{formatNumericDisplayValue(value)}</dd>
        </div>
      ))}
    </dl>
  )
}

export function SystemTasksPage({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<SystemTaskSearch>) => void
  search: SystemTaskSearch
  siteId?: string
}) {
  const { t } = useTranslation()
  const [initialJob, setInitialJob] = useState<StatisticsExportJobItem>()
  const validSiteId = siteId == null || isIdString(siteId)
  const parsedSiteId =
    siteId && isIdString(siteId) ? parseIdString(siteId) : undefined
  const currentParams = useMemo(() => params(search), [search])
  const listQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      parsedSiteId
        ? listSiteSystemTasks(parsedSiteId, currentParams)
        : listSystemTasks(currentParams),
    queryKey: parsedSiteId
      ? systemTaskKeys.site(siteId ?? '', 'list', currentParams)
      : systemTaskKeys.global('list', currentParams),
  })
  const statisticsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      parsedSiteId
        ? getSiteSystemTaskStatistics(parsedSiteId, currentParams)
        : getSystemTaskStatistics(currentParams),
    queryKey: parsedSiteId
      ? systemTaskKeys.site(siteId ?? '', 'statistics', currentParams)
      : systemTaskKeys.global('statistics', currentParams),
  })
  const exportMutation = useMutation({
    mutationFn: (format: StatisticsExportFormat) =>
      createStatisticsExport(
        buildSystemTaskExportRequest(format, search, parsedSiteId)
      ),
    onError: (error) =>
      toast.error(t(dynamicI18nKey('api', getApiErrorTranslationKey(error)))),
    onSuccess: (job) => {
      setInitialJob(job)
      onSearchChange({ exportId: job.id })
    },
  })
  const columns = useMemo<ColumnDef<SystemTaskItem, unknown>[]>(
    () => [
      {
        id: 'identity',
        header: t('systemTasks.identity'),
        cell: ({ row }) => (
          <div className='grid min-w-48 gap-1'>
            <strong>{row.original.task_id}</strong>
            <span className='text-muted-foreground text-xs'>
              {row.original.site_name} · {row.original.site_id}
            </span>
            <code className='text-muted-foreground text-xs'>
              {row.original.remote_id}
            </code>
            <Badge variant='neutral'>
              {taskTypeText(t, row.original.type)}
            </Badge>
          </div>
        ),
      },
      {
        id: 'status',
        header: t('common.status'),
        cell: ({ row }) => (
          <div className='grid min-w-32 gap-1'>
            <StatusBadge status={row.original.status} />
            {row.original.error_present && (
              <Badge variant='destructive'>
                {errorCodeText(t, row.original.error_code)}
              </Badge>
            )}
            <DataStatusBadge status={row.original.data_status} />
          </div>
        ),
      },
      {
        id: 'progress',
        header: t('systemTasks.progress.title'),
        cell: ({ row }) => <ProgressView item={row.original} />,
      },
      {
        id: 'result',
        header: t('systemTasks.result.title'),
        cell: ({ row }) => <ResultView item={row.original} />,
      },
      {
        id: 'time',
        header: t('systemTasks.timestamps'),
        cell: ({ row }) => (
          <div className='grid min-w-40 gap-1 text-xs'>
            <span>{timestamp(row.original.remote_created_at)}</span>
            <span>{timestamp(row.original.remote_updated_at)}</span>
            <span>{timestamp(row.original.collected_at)}</span>
          </div>
        ),
      },
    ],
    [t]
  )
  const data = listQuery.data
  const hasNext = data
    ? BigInt(data.total) > BigInt(search.page) * BigInt(search.pageSize)
    : false
  let approximateTotal = 0
  if (data) {
    approximateTotal = hasNext
      ? search.page * search.pageSize + 1
      : (search.page - 1) * search.pageSize + data.items.length
  }
  const stats = statisticsQuery.data
  return (
    <SectionPageLayout
      actions={(['xlsx', 'csv'] as const).map((format) => (
        <Button
          disabled={exportMutation.isPending || !validSiteId}
          key={format}
          onClick={() => exportMutation.mutate(format)}
          variant='outline'
        >
          <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
          {t('systemTasks.export', { format: format.toUpperCase() })}
        </Button>
      ))}
      description={
        siteId
          ? t('systemTasks.siteDescription', { id: siteId })
          : t('systemTasks.description')
      }
      title={siteId ? t('systemTasks.siteTitle') : t('systemTasks.title')}
    >
      <div className='grid min-w-0 gap-6'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('systemTasks.backToSite')}
          </DetailBackLink>
        )}
        <section
          className='border-primary/30 bg-primary/5 rounded-lg border p-4'
          role='note'
        >
          <p className='font-medium'>{t('systemTasks.boundary.title')}</p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('systemTasks.boundary.description')}
          </p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('systemTasks.retention')}
          </p>
        </section>
        <Filters global={!siteId} onChange={onSearchChange} search={search} />
        {stats && (
          <div className='grid gap-5'>
            <div className='flex items-center gap-2' role='status'>
              <span>{t('systemTasks.statisticsStatus')}</span>
              <DataStatusBadge status={stats.data_status} />
              <span className='text-muted-foreground text-xs'>
                {timestamp(stats.as_of)}
              </span>
            </div>
            <MetricGrid metric={stats.summary} />
            <div className='grid gap-5 xl:grid-cols-3'>
              <Breakdown
                items={stats.type_breakdown}
                title={t('systemTasks.breakdown.type')}
              />
              <Breakdown
                items={stats.status_breakdown}
                title={t('systemTasks.breakdown.status')}
              />
              <Breakdown
                items={stats.site_breakdown}
                title={t('systemTasks.breakdown.site')}
              />
            </div>
          </div>
        )}
        {data?.truncated && (
          <section
            className='border-warning/40 bg-warning/10 rounded-lg border p-4'
            role='alert'
          >
            <p className='font-medium'>{t('systemTasks.truncation.title')}</p>
            <p className='text-sm'>
              {data.truncation_reason === 'id_gap'
                ? t('systemTasks.truncation.id_gap', {
                    count: data.observed_count,
                  })
                : t('systemTasks.truncation.source_limit', {
                    count: data.observed_count,
                    limit: data.source_limit,
                  })}
            </p>
          </section>
        )}
        <div className='flex flex-wrap items-center gap-2' role='status'>
          <span>{t('systemTasks.listStatus')}</span>
          <DataStatusBadge status={data?.data_status ?? 'pending'} />
          {data && (
            <span className='text-muted-foreground text-xs'>
              {t('systemTasks.totalValue', { total: data.total })} ·{' '}
              {timestamp(data.as_of)}
            </span>
          )}
        </div>
        <DataTable
          ariaLabel={t('systemTasks.table')}
          columns={columns}
          data={data?.items ?? []}
          emptyDescription={t('systemTasks.emptyDescription')}
          emptyTitle={t('systemTasks.empty')}
          error={!validSiteId || listQuery.isError}
          fetching={listQuery.isFetching}
          loading={listQuery.isPending}
          onPageChange={(page) => onSearchChange({ page })}
          onPageSizeChange={(pageSize) => onSearchChange({ page: 1, pageSize })}
          onRetry={() => void listQuery.refetch()}
          page={search.page}
          pageSize={search.pageSize}
          renderMobileCard={(item) => (
            <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
              <div className='flex items-start justify-between gap-2'>
                <div>
                  <strong>{item.task_id}</strong>
                  <p className='text-muted-foreground text-xs'>
                    {item.site_name} · {item.site_id} · {item.remote_id}
                  </p>
                </div>
                <StatusBadge status={item.status} />
              </div>
              <Badge variant='neutral'>{taskTypeText(t, item.type)}</Badge>
              <ProgressView item={item} />
              <ResultView item={item} />
              {item.error_present && (
                <Badge variant='destructive'>
                  {errorCodeText(t, item.error_code)}
                </Badge>
              )}
              <DataStatusBadge status={item.data_status} />
            </article>
          )}
          total={approximateTotal}
        />
      </div>
      <ExportTaskSheet
        exportId={search.exportId}
        initialJob={initialJob}
        onOpenChange={(open) =>
          !open && onSearchChange({ exportId: undefined })
        }
        onRecreate={(job) => exportMutation.mutate(job.format)}
        recreating={exportMutation.isPending}
      />
    </SectionPageLayout>
  )
}
