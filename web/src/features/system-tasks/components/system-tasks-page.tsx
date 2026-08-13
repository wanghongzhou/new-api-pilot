import {
  Alert02Icon,
  ArrowLeft01Icon,
  Chart01Icon,
  Database01Icon,
  FileExportIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataStatusBadge } from '@/components/data/data-status'
import { FacetedFilter } from '@/components/data/faceted-filter'
import { MetricValue } from '@/components/data/metric-value'
import { QueryStateAlert } from '@/components/data/query-state-alert'
import { ErrorState } from '@/components/error-state'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { listSites } from '@/features/sites/api'
import { siteKeys } from '@/features/sites/query-keys'
import type { SiteListItem } from '@/features/sites/types'
import { createStatisticsExport } from '@/features/statistics/api'
import { ExportTaskSheet } from '@/features/statistics/components/export-task-sheet'
import type {
  StatisticsExportFormat,
  StatisticsExportJobItem,
} from '@/features/statistics/types'
import { useRetainedQueryData } from '@/hooks/use-retained-query-data'
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
import {
  buildSystemTaskSearch,
  changeSystemTaskTab,
  type SystemTaskSearch,
} from '../search'
import {
  systemTaskStatuses,
  systemTaskTypes,
  type SystemTaskBreakdown,
  type SystemTaskItem,
  type SystemTaskMetric,
  type SystemTaskPage,
  type SystemTaskQueryParams,
  type SystemTaskStatistics,
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
  if (type === 'log_detail_cleanup') {
    return t('systemTasks.type.log_detail_cleanup')
  }
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
function StatusBadge({
  className,
  status,
}: {
  className?: string
  status: SystemTaskStatus
}) {
  const { t } = useTranslation()
  let variant: 'destructive' | 'primary' | 'success' | 'warning' = 'warning'
  if (status === 'succeeded') variant = 'success'
  else if (status === 'failed') variant = 'destructive'
  else if (status === 'running') variant = 'primary'
  return (
    <Badge className={className} variant={variant}>
      {taskStatusText(t, status)}
    </Badge>
  )
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
    {
      icon: Database01Icon,
      label: t('systemTasks.metric.total'),
      value: metric.total,
    },
    {
      icon: Chart01Icon,
      label: t('systemTasks.metric.active'),
      value: metric.active,
    },
    {
      icon: Chart01Icon,
      label: t('systemTasks.metric.succeeded'),
      value: metric.succeeded,
    },
    {
      icon: Alert02Icon,
      label: t('systemTasks.metric.failed'),
      value: metric.failed,
    },
    {
      icon: Alert02Icon,
      label: t('systemTasks.metric.errorPresent'),
      value: metric.error_present,
    },
  ] as const
  return (
    <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-5'>
      {values.map(({ icon, label, value }) => (
        <div
          className='bg-card text-card-foreground ring-foreground/10 flex items-center gap-3 rounded-xl p-4 ring-1'
          key={label}
        >
          <span className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg'>
            <HugeiconsIcon icon={icon} size={18} strokeWidth={2} />
          </span>
          <dl className='min-w-0'>
            <dt className='text-muted-foreground truncate text-xs'>{label}</dt>
            <dd className='mt-0.5 text-2xl font-semibold tracking-tight'>
              <MetricValue value={value} />
            </dd>
          </dl>
        </div>
      ))}
    </div>
  )
}

function Breakdown({
  items,
  title,
}: {
  items: SystemTaskBreakdown[]
  title: string
}) {
  const { t } = useTranslation()
  return (
    <section className='grid content-start gap-2'>
      <h2 className='font-semibold'>{title}</h2>
      {items.length === 0 ? (
        <p className='text-muted-foreground text-sm'>{t('common.none')}</p>
      ) : (
        <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
          {items.map((item) => (
            <article
              className='border-border grid gap-3 rounded-lg border p-3'
              key={`${item.dimension_id}:${item.site_id}`}
            >
              <div className='flex items-start justify-between gap-2'>
                <div className='min-w-0'>
                  <p className='truncate font-medium'>
                    {item.dimension_name || item.site_name}
                  </p>
                  <code className='text-muted-foreground text-xs'>
                    {item.dimension_id || item.site_id}
                  </code>
                </div>
                <DataStatusBadge status={item.data_status} />
              </div>
              <dl className='grid grid-cols-2 gap-x-4 gap-y-2 text-sm'>
                {[
                  { label: t('systemTasks.metric.total'), value: item.total },
                  { label: t('systemTasks.metric.active'), value: item.active },
                  {
                    label: t('systemTasks.metric.succeeded'),
                    value: item.succeeded,
                  },
                  { label: t('systemTasks.metric.failed'), value: item.failed },
                  {
                    label: t('systemTasks.metric.errorPresent'),
                    value: item.error_present,
                  },
                ].map(({ label, value }) => (
                  <div key={label}>
                    <dt className='text-muted-foreground text-xs'>{label}</dt>
                    <dd className='font-mono font-medium'>
                      <MetricValue value={value} />
                    </dd>
                  </div>
                ))}
              </dl>
              <span className='text-muted-foreground text-xs'>
                {timestamp(item.as_of)}
              </span>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

function TabPurpose({
  dataErrorCode,
  metadata,
  notice,
  status,
  tab,
}: {
  dataErrorCode: SystemTaskStatistics['data_error_code'] | undefined
  metadata?: string
  notice?: string
  status: SystemTaskStatistics['data_status'] | undefined
  tab: SystemTaskSearch['tab']
}) {
  const { t } = useTranslation()
  const keys = {
    list: 'list',
    sites: 'sites',
    statuses: 'statuses',
    types: 'types',
  } as const
  const key = keys[tab]
  return (
    <section className='border-border bg-muted/30 flex flex-wrap items-start justify-between gap-3 rounded-xl border p-4'>
      <div className='min-w-0'>
        <p className='font-medium'>
          {t(dynamicI18nKey('systemTasks', `systemTasks.purpose.${key}Title`))}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            dynamicI18nKey(
              'systemTasks',
              `systemTasks.purpose.${key}Description`
            )
          )}
        </p>
        {metadata && (
          <p className='text-muted-foreground mt-1 text-xs'>{metadata}</p>
        )}
        {notice && (
          <p className='text-warning mt-1 text-xs' role='alert'>
            {notice}
          </p>
        )}
      </div>
      <div className='flex items-center gap-2'>
        <DataStatusBadge status={status ?? 'pending'} />
        {dataErrorCode && (
          <span className='text-destructive text-xs' role='alert'>
            {t(
              dynamicI18nKey(
                'systemTasks',
                `systemTasks.dataError.${dataErrorCode}`
              )
            )}
          </span>
        )}
      </div>
    </section>
  )
}

function Filters({
  global,
  onChange,
  search,
  sites,
}: {
  global: boolean
  onChange: (changes: Partial<SystemTaskSearch>) => void
  search: SystemTaskSearch
  sites: SiteListItem[]
}) {
  const { t } = useTranslation()
  const reset = buildSystemTaskSearch({
    pageSize: search.pageSize,
    tab: search.tab,
  })
  const hasActiveFilters = hasFilterChanges(search, reset, [
    'createdEnd',
    'createdStart',
    'errorPresent',
    'siteIds',
    'statuses',
    'types',
  ])
  let errorValue = ''
  if (search.errorPresent === true) errorValue = 'yes'
  if (search.errorPresent === false) errorValue = 'no'
  return (
    <section
      aria-label={t('systemTasks.filters.title')}
      className='flex min-w-0 flex-wrap items-center gap-2'
    >
      {global && (
        <FacetedFilter
          clearLabel={t('systemTasks.filters.allSites')}
          onChange={(value) =>
            onChange({
              page: 1,
              siteIds: isIdString(value) ? [parseIdString(value)] : [],
            })
          }
          options={sites.map((site) => ({ label: site.name, value: site.id }))}
          title={t('systemTasks.filters.site')}
          value={search.siteIds.length === 1 ? search.siteIds[0] : ''}
        />
      )}
      <FacetedFilter
        clearLabel={t('systemTasks.filters.allTypes')}
        onChange={(value) =>
          onChange({
            page: 1,
            types: systemTaskTypes.includes(value as SystemTaskType)
              ? [value as SystemTaskType]
              : [],
          })
        }
        options={systemTaskTypes.map((type) => ({
          label: taskTypeText(t, type),
          value: type,
        }))}
        title={t('systemTasks.filters.types')}
        value={search.types.length === 1 ? search.types[0] : ''}
      />
      <FacetedFilter
        clearLabel={t('systemTasks.filters.allStatuses')}
        onChange={(value) =>
          onChange({
            page: 1,
            statuses: systemTaskStatuses.includes(value as SystemTaskStatus)
              ? [value as SystemTaskStatus]
              : [],
          })
        }
        options={systemTaskStatuses.map((status) => ({
          label: taskStatusText(t, status),
          value: status,
        }))}
        title={t('systemTasks.filters.statuses')}
        value={search.statuses.length === 1 ? search.statuses[0] : ''}
      />
      <FacetedFilter
        clearLabel={t('systemTasks.filters.allErrors')}
        onChange={(value) => {
          let errorPresent: boolean | undefined
          if (value === 'yes') errorPresent = true
          if (value === 'no') errorPresent = false
          onChange({
            errorPresent,
            page: 1,
          })
        }}
        options={[
          { label: errorFilterText(t, true), value: 'yes' },
          { label: errorFilterText(t, false), value: 'no' },
        ]}
        title={t('systemTasks.filters.errorPresent')}
        value={errorValue}
      />
      <label>
        <span className='sr-only'>{t('systemTasks.filters.createdStart')}</span>
        <Input
          aria-label={t('systemTasks.filters.createdStart')}
          className='h-10 w-48 sm:h-8'
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
      <label>
        <span className='sr-only'>{t('systemTasks.filters.createdEnd')}</span>
        <Input
          aria-label={t('systemTasks.filters.createdEnd')}
          className='h-10 w-48 sm:h-8'
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
      {hasActiveFilters && (
        <Button
          className='text-muted-foreground px-2'
          onClick={() => onChange(reset)}
          size='sm'
          type='button'
          variant='ghost'
        >
          {t('common.reset')}
        </Button>
      )}
    </section>
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
  if (item.type === 'log_detail_cleanup') {
    entries.push([
      t('systemTasks.result.archivedDays'),
      item.result.archived_days,
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
  const overviewParams = useMemo(() => params(buildSystemTaskSearch({})), [])
  const siteParams = useMemo(
    () => ({
      p: 1,
      page_size: 100,
      sort_by: 'name',
      sort_order: 'asc' as const,
    }),
    []
  )
  const sitesQuery = useQuery({
    enabled: siteId == null,
    queryFn: () => listSites(siteParams),
    queryKey: siteKeys.list(siteParams),
    staleTime: 5 * 60_000,
  })
  const listQuery = useQuery({
    enabled: validSiteId && search.tab === 'list',
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
        ? getSiteSystemTaskStatistics(parsedSiteId, overviewParams)
        : getSystemTaskStatistics(overviewParams),
    queryKey: parsedSiteId
      ? systemTaskKeys.site(siteId ?? '', 'statistics', overviewParams)
      : systemTaskKeys.global('statistics', overviewParams),
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
  const retainedScope = siteId ? `site:${siteId}` : 'global'
  const data = useRetainedQueryData(
    listQuery.data,
    listQuery.isError,
    retainedScope
  )
  const hasNext = data
    ? BigInt(data.total) > BigInt(search.page) * BigInt(search.pageSize)
    : false
  let approximateTotal = 0
  if (data) {
    approximateTotal = hasNext
      ? search.page * search.pageSize + 1
      : (search.page - 1) * search.pageSize + data.items.length
  }
  const stats = useRetainedQueryData(
    statisticsQuery.data,
    statisticsQuery.isError,
    retainedScope
  )
  const unavailable = data?.data_status === 'unavailable'
  const emptyTitleKey = unavailable
    ? 'systemTasks.empty.unavailableTitle'
    : 'systemTasks.empty'
  const emptyDescriptionKey = unavailable
    ? 'systemTasks.empty.unavailableDescription'
    : 'systemTasks.emptyDescription'
  let activeBreakdown = stats?.site_breakdown ?? []
  let activeBreakdownTitle = t('systemTasks.breakdown.site')
  if (search.tab === 'types') {
    activeBreakdown = stats?.type_breakdown ?? []
    activeBreakdownTitle = t('systemTasks.breakdown.type')
  } else if (search.tab === 'statuses') {
    activeBreakdown = stats?.status_breakdown ?? []
    activeBreakdownTitle = t('systemTasks.breakdown.status')
  }
  const tabs = [
    {
      count: stats?.summary.total,
      icon: Database01Icon,
      label: t('systemTasks.tabs.list'),
      value: 'list',
    },
    {
      count: stats?.type_breakdown.length,
      icon: Chart01Icon,
      label: t('systemTasks.tabs.types'),
      value: 'types',
    },
    {
      count: stats?.status_breakdown.length,
      icon: Alert02Icon,
      label: t('systemTasks.tabs.statuses'),
      value: 'statuses',
    },
    {
      count: stats?.site_breakdown.length,
      icon: Database01Icon,
      label: t('systemTasks.tabs.sites'),
      value: 'sites',
    },
  ] as const
  let truncation: SystemTaskPage | SystemTaskStatistics | undefined
  if (data?.truncated) truncation = data
  else if (!data && stats?.truncated) truncation = stats
  let truncationMessage: string | undefined
  if (truncation?.truncation_reason === 'id_gap') {
    truncationMessage = t('systemTasks.truncation.id_gap', {
      count: truncation.observed_count,
    })
  } else if (truncation?.truncation_reason === 'source_limit_and_id_gap') {
    truncationMessage = t('systemTasks.truncation.source_limit_and_id_gap', {
      count: truncation.observed_count,
      limit: truncation.source_limit,
    })
  } else if (truncation) {
    truncationMessage = t('systemTasks.truncation.source_limit', {
      count: truncation.observed_count,
      limit: truncation.source_limit,
    })
  }
  let purposeMetadata: string | undefined
  if (search.tab === 'list' && data) {
    purposeMetadata = `${t('systemTasks.totalValue', {
      total: data.total,
    })} · ${t('systemTasks.asOf', { time: timestamp(data.as_of) })}`
  } else if (stats) {
    purposeMetadata = t('systemTasks.asOf', { time: timestamp(stats.as_of) })
  }
  return (
    <SectionPageLayout
      actions={
        search.tab === 'list'
          ? (['xlsx', 'csv'] as const).map((format) => (
              <Button
                disabled={exportMutation.isPending || !validSiteId}
                key={format}
                onClick={() => exportMutation.mutate(format)}
                variant='outline'
              >
                <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
                {t('systemTasks.export', { format: format.toUpperCase() })}
              </Button>
            ))
          : undefined
      }
      description={
        siteId
          ? t('systemTasks.siteDescription', { id: siteId })
          : t('systemTasks.description')
      }
      fixedContent
      mobileScrollableContent
      title={siteId ? t('systemTasks.siteTitle') : t('systemTasks.title')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('systemTasks.backToSite')}
          </DetailBackLink>
        )}
        {stats ? (
          <MetricGrid metric={stats.summary} />
        ) : (
          <div
            aria-hidden='true'
            className='border-border bg-muted/40 h-20 animate-pulse rounded-lg border'
          />
        )}
        <Tabs
          onValueChange={(tab) =>
            onSearchChange(changeSystemTaskTab(tab as SystemTaskSearch['tab']))
          }
          value={search.tab}
        >
          <TabsList
            aria-label={t('systemTasks.tabs.label')}
            className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'
          >
            {tabs.map((tab) => (
              <TabsTrigger key={tab.value} value={tab.value}>
                <HugeiconsIcon icon={tab.icon} size={15} strokeWidth={2} />
                {tab.label}
                {tab.count != null && (
                  <Badge className='px-1.5 font-mono' variant='secondary'>
                    {tab.count}
                  </Badge>
                )}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <TabPurpose
          dataErrorCode={data?.data_error_code ?? stats?.data_error_code}
          metadata={purposeMetadata}
          notice={truncationMessage}
          status={data?.data_status ?? stats?.data_status}
          tab={search.tab}
        />
        {search.tab === 'list' && (
          <Filters
            global={!siteId}
            onChange={onSearchChange}
            search={search}
            sites={sitesQuery.data?.items ?? []}
          />
        )}
        {!siteId && sitesQuery.isError && search.tab === 'list' && (
          <QueryStateAlert
            message={t('common.siteOptionsRefreshFailed')}
            onRetry={() => void sitesQuery.refetch()}
          />
        )}
        {listQuery.isError && data && search.tab === 'list' && (
          <QueryStateAlert
            message={t('common.retainedDataRefreshFailed')}
            onRetry={() => void listQuery.refetch()}
          />
        )}
        {statisticsQuery.isError && stats && (
          <QueryStateAlert
            message={t('common.retainedDataRefreshFailed')}
            onRetry={() => void statisticsQuery.refetch()}
          />
        )}
        {statisticsQuery.isError && !stats && (
          <ErrorState
            className='min-h-40'
            onRetry={
              validSiteId ? () => void statisticsQuery.refetch() : undefined
            }
            title={t('systemTasks.statisticsError')}
          />
        )}
        {search.tab === 'list' && (
          <DataTable
            ariaLabel={t('systemTasks.table')}
            columns={columns}
            data={data?.items ?? []}
            emptyDescription={t(
              dynamicI18nKey('systemTasks', emptyDescriptionKey)
            )}
            emptyTitle={t(dynamicI18nKey('systemTasks', emptyTitleKey))}
            error={!validSiteId || (listQuery.isError && !data)}
            fetching={listQuery.isFetching}
            loading={listQuery.isPending}
            mobileCardBreakpoint='wide'
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={validSiteId ? () => void listQuery.refetch() : undefined}
            page={search.page}
            pageSize={search.pageSize}
            paginationHasKnownLastPage={false}
            paginationHasNextPage={hasNext}
            paginationTotalDisplay={data?.total ?? '0'}
            renderMobileCard={(item) => (
              <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
                <div className='flex items-start justify-between gap-2'>
                  <div className='min-w-0'>
                    <strong className='block break-all'>{item.task_id}</strong>
                    <p className='text-foreground text-xs'>
                      {item.site_name} · {item.site_id} · {item.remote_id}
                    </p>
                  </div>
                  <StatusBadge
                    className='bg-muted text-foreground'
                    status={item.status}
                  />
                </div>
                <Badge className='bg-muted text-foreground' variant='neutral'>
                  {taskTypeText(t, item.type)}
                </Badge>
                <ProgressView item={item} />
                <ResultView item={item} />
                {item.error_present && (
                  <Badge variant='destructive'>
                    {errorCodeText(t, item.error_code)}
                  </Badge>
                )}
                <DataStatusBadge
                  className='bg-muted text-foreground'
                  status={item.data_status}
                />
                <dl className='border-border grid gap-2 border-t pt-3 text-xs'>
                  <div>
                    <dt className='text-foreground'>
                      {t('systemTasks.time.created')}
                    </dt>
                    <dd>{timestamp(item.remote_created_at)}</dd>
                  </div>
                  <div>
                    <dt className='text-foreground'>
                      {t('systemTasks.time.updated')}
                    </dt>
                    <dd>{timestamp(item.remote_updated_at)}</dd>
                  </div>
                  <div>
                    <dt className='text-foreground'>
                      {t('systemTasks.time.collected')}
                    </dt>
                    <dd>{timestamp(item.collected_at)}</dd>
                  </div>
                </dl>
              </article>
            )}
            total={approximateTotal}
          />
        )}
        {stats && search.tab !== 'list' && (
          <div className='min-h-0 flex-1 overflow-y-auto' tabIndex={0}>
            <Breakdown items={activeBreakdown} title={activeBreakdownTitle} />
          </div>
        )}
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
