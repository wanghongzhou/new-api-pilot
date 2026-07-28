import {
  Alert02Icon,
  ArrowLeft01Icon,
  Chart01Icon,
  Database01Icon,
  FileExportIcon,
  Search01Icon,
  Settings02Icon,
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
import { ErrorState } from '@/components/error-state'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
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
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import {
  isIdString,
  isNonNegativeIdString,
  parseIdString,
  parseNonNegativeIdString,
} from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'
import { hasFilterChanges } from '@/lib/filter-state'

import {
  getSiteUpstreamTaskStatistics,
  getUpstreamTaskStatistics,
  listSiteUpstreamTasks,
  listUpstreamTasks,
} from '../api'
import { buildUpstreamTaskExportRequest } from '../export-request'
import { upstreamTaskKeys } from '../query-keys'
import {
  buildUpstreamTaskSearch,
  changeUpstreamTaskTab,
  upstreamTaskStatuses,
  type UpstreamTaskSearch,
} from '../search'
import type {
  UpstreamTaskBreakdown,
  UpstreamTaskItem,
  UpstreamTaskMetric,
  UpstreamTaskQueryParams,
  UpstreamTaskStatus,
} from '../types'

function params(search: UpstreamTaskSearch): UpstreamTaskQueryParams {
  return {
    actions: search.actions,
    end_timestamp: search.end,
    groups: search.groups,
    models: search.models,
    p: search.page,
    page_size: search.pageSize,
    platforms: search.platforms,
    remote_channel_id: search.remoteChannelId,
    remote_id: search.remoteId,
    remote_user_id: search.remoteUserId,
    site_ids: search.siteIds,
    start_timestamp: search.start,
    statuses: search.statuses,
    task_id: search.taskId || undefined,
  }
}

function timestamp(value: number | null) {
  if (value == null || value <= 0) return '-'
  return fromUnixSeconds(value).format('YYYY-MM-DD HH:mm:ss')
}

function dateTimeValue(value?: number) {
  return value == null ? '' : fromUnixSeconds(value).format('YYYY-MM-DDTHH:mm')
}

function parseDateTime(value: string) {
  if (!value) return undefined
  const parsed = dayjs.tz(value, 'YYYY-MM-DDTHH:mm', BEIJING_TIMEZONE)
  return parsed.isValid() ? parsed.unix() : undefined
}

function statusText(t: (key: string) => string, status: UpstreamTaskStatus) {
  if (status === 'NOT_START') return t('upstreamTasks.status.NOT_START')
  if (status === 'SUBMITTED') return t('upstreamTasks.status.SUBMITTED')
  if (status === 'QUEUED') return t('upstreamTasks.status.QUEUED')
  if (status === 'IN_PROGRESS') return t('upstreamTasks.status.IN_PROGRESS')
  if (status === 'FAILURE') return t('upstreamTasks.status.FAILURE')
  if (status === 'SUCCESS') return t('upstreamTasks.status.SUCCESS')
  return t('upstreamTasks.status.UNKNOWN')
}

function TaskStatusBadge({ status }: { status: UpstreamTaskStatus }) {
  const { t } = useTranslation()
  let variant: 'destructive' | 'neutral' | 'primary' | 'success' | 'warning' =
    'neutral'
  if (status === 'SUCCESS') variant = 'success'
  else if (status === 'FAILURE') variant = 'destructive'
  else if (status === 'IN_PROGRESS') variant = 'primary'
  else if (status === 'QUEUED' || status === 'SUBMITTED') variant = 'warning'
  return <Badge variant={variant}>{statusText(t, status)}</Badge>
}

function MetricGrid({ metric }: { metric: UpstreamTaskMetric }) {
  const { t } = useTranslation()
  const primary = [
    {
      icon: Database01Icon,
      label: t('upstreamTasks.metric.total'),
      value: metric.total,
    },
    {
      icon: Chart01Icon,
      label: t('upstreamTasks.metric.queued'),
      value: metric.queued,
    },
    {
      icon: Chart01Icon,
      label: t('upstreamTasks.metric.running'),
      value: metric.running,
    },
    {
      icon: Chart01Icon,
      label: t('upstreamTasks.metric.success'),
      value: metric.success,
    },
    {
      icon: Alert02Icon,
      label: t('upstreamTasks.metric.failure'),
      value: metric.failure,
    },
  ] as const
  const quality = [
    {
      label: t('upstreamTasks.metric.successRate'),
      value: metric.success_rate,
    },
    {
      label: t('upstreamTasks.metric.avgQueue'),
      value: metric.avg_queue_seconds,
    },
    {
      label: t('upstreamTasks.metric.avgRun'),
      value: metric.avg_run_seconds,
    },
    {
      label: t('upstreamTasks.metric.avgTotal'),
      value: metric.avg_total_seconds,
    },
  ] as const
  return (
    <div className='grid gap-3'>
      <dl className='grid gap-3 sm:grid-cols-2 xl:grid-cols-5'>
        {primary.map(({ icon, label, value }) => (
          <div
            className='bg-card text-card-foreground ring-foreground/10 flex min-w-0 items-center gap-3 rounded-xl p-4 ring-1'
            key={label}
          >
            <span className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg'>
              <HugeiconsIcon icon={icon} size={18} strokeWidth={2} />
            </span>
            <div className='min-w-0'>
              <dt className='text-muted-foreground truncate text-xs'>
                {label}
              </dt>
              <dd className='mt-0.5 text-2xl font-semibold tracking-tight'>
                <MetricValue value={value} />
              </dd>
            </div>
          </div>
        ))}
      </dl>
      <dl className='border-border bg-muted/20 grid gap-x-6 gap-y-2 rounded-xl border px-4 py-3 sm:grid-cols-2 xl:grid-cols-4'>
        {quality.map(({ label, value }) => (
          <div
            className='flex min-w-0 items-baseline justify-between gap-3'
            key={label}
          >
            <dt className='text-muted-foreground truncate text-xs'>{label}</dt>
            <dd className='shrink-0 font-mono text-sm font-medium'>
              <MetricValue value={value} />
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function Filters({
  global,
  onChange,
  search,
  sites,
}: {
  global: boolean
  onChange: (changes: Partial<UpstreamTaskSearch>) => void
  search: UpstreamTaskSearch
  sites: SiteListItem[]
}) {
  const { t } = useTranslation()
  const reset = buildUpstreamTaskSearch({
    pageSize: search.pageSize,
    tab: search.tab,
  })
  const advancedTextFilter = (
    key: 'actions' | 'groups' | 'models' | 'platforms',
    label: string
  ) => (
    <label className='grid gap-1.5'>
      <span className='text-muted-foreground text-xs'>{label}</span>
      <Input
        aria-label={label}
        className='h-8'
        onChange={(event) =>
          onChange({
            [key]: event.target.value.trim() ? [event.target.value] : [],
            page: 1,
          })
        }
        value={search[key].length === 1 ? search[key][0] : ''}
      />
    </label>
  )
  const advancedCount = [
    search.remoteChannelId != null,
    search.remoteId != null,
    search.remoteUserId != null,
    search.platforms.length > 0,
    search.groups.length > 0,
    search.actions.length > 0,
    search.models.length > 0,
    search.start != null,
    search.end != null,
  ].filter(Boolean).length
  const hasActiveFilters = hasFilterChanges(search, reset, [
    'actions',
    'end',
    'groups',
    'models',
    'platforms',
    'remoteChannelId',
    'remoteId',
    'remoteUserId',
    'siteIds',
    'start',
    'statuses',
    'taskId',
  ])
  return (
    <section
      aria-label={t('upstreamTasks.filters.title')}
      className='flex min-w-0 flex-wrap items-center gap-2'
    >
      <label className='relative min-w-48 flex-1 sm:max-w-72'>
        <span className='sr-only'>{t('upstreamTasks.filters.taskId')}</span>
        <HugeiconsIcon
          className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2'
          icon={Search01Icon}
          size={15}
          strokeWidth={2}
        />
        <Input
          aria-label={t('upstreamTasks.filters.taskId')}
          className='h-10 pl-8 sm:h-8'
          onChange={(event) =>
            onChange({ page: 1, taskId: event.target.value })
          }
          placeholder={t('upstreamTasks.filters.taskId')}
          value={search.taskId}
        />
      </label>
      {global && (
        <FacetedFilter
          clearLabel={t('upstreamTasks.filters.allSites')}
          onChange={(value) =>
            onChange({
              page: 1,
              siteIds: isIdString(value) ? [parseIdString(value)] : [],
            })
          }
          options={sites.map((site) => ({ label: site.name, value: site.id }))}
          title={t('upstreamTasks.filters.site')}
          value={search.siteIds.length === 1 ? search.siteIds[0] : ''}
        />
      )}
      <FacetedFilter
        clearLabel={t('upstreamTasks.filters.allStatuses')}
        onChange={(value) =>
          onChange({
            page: 1,
            statuses: upstreamTaskStatuses.includes(value as UpstreamTaskStatus)
              ? [value as UpstreamTaskStatus]
              : [],
          })
        }
        options={upstreamTaskStatuses.map((status) => ({
          label: statusText(t, status),
          value: status,
        }))}
        title={t('upstreamTasks.filters.statuses')}
        value={search.statuses.length === 1 ? search.statuses[0] : ''}
      />
      <Popover>
        <PopoverTrigger
          render={
            <Button
              className='h-10 border-dashed sm:h-8'
              size='sm'
              type='button'
              variant='outline'
            />
          }
        >
          <HugeiconsIcon icon={Settings02Icon} size={15} strokeWidth={2} />
          {t('common.moreFilters')}
          {advancedCount > 0 && (
            <Badge className='px-1.5 font-mono' variant='secondary'>
              {advancedCount}
            </Badge>
          )}
        </PopoverTrigger>
        <PopoverContent
          align='start'
          className='w-[min(640px,calc(100vw-2rem))] p-3'
        >
          <div className='grid gap-3 sm:grid-cols-2'>
            <label className='grid gap-1.5'>
              <span className='text-muted-foreground text-xs'>
                {t('upstreamTasks.filters.remoteId')}
              </span>
              <Input
                aria-label={t('upstreamTasks.filters.remoteId')}
                className='h-8'
                inputMode='numeric'
                onChange={(event) => {
                  const value = event.target.value
                  onChange({
                    page: 1,
                    remoteId: isIdString(value)
                      ? parseIdString(value)
                      : undefined,
                  })
                }}
                value={search.remoteId ?? ''}
              />
            </label>
            <label className='grid gap-1.5'>
              <span className='text-muted-foreground text-xs'>
                {t('upstreamTasks.filters.remoteUserId')}
              </span>
              <Input
                aria-label={t('upstreamTasks.filters.remoteUserId')}
                className='h-8'
                inputMode='numeric'
                onChange={(event) => {
                  const value = event.target.value
                  onChange({
                    page: 1,
                    remoteUserId: isNonNegativeIdString(value)
                      ? parseNonNegativeIdString(value)
                      : undefined,
                  })
                }}
                value={search.remoteUserId ?? ''}
              />
            </label>
            <label className='grid gap-1.5'>
              <span className='text-muted-foreground text-xs'>
                {t('upstreamTasks.filters.remoteChannelId')}
              </span>
              <Input
                aria-label={t('upstreamTasks.filters.remoteChannelId')}
                className='h-8'
                inputMode='numeric'
                onChange={(event) => {
                  const value = event.target.value
                  onChange({
                    page: 1,
                    remoteChannelId: isNonNegativeIdString(value)
                      ? parseNonNegativeIdString(value)
                      : undefined,
                  })
                }}
                value={search.remoteChannelId ?? ''}
              />
            </label>
            {advancedTextFilter(
              'platforms',
              t('upstreamTasks.filters.platforms')
            )}
            {advancedTextFilter('groups', t('upstreamTasks.filters.groups'))}
            {advancedTextFilter('actions', t('upstreamTasks.filters.actions'))}
            {advancedTextFilter('models', t('upstreamTasks.filters.models'))}
            <label className='grid gap-1.5'>
              <span className='text-muted-foreground text-xs'>
                {t('upstreamTasks.filters.start')}
              </span>
              <Input
                aria-label={t('upstreamTasks.filters.start')}
                className='h-8'
                onChange={(event) =>
                  onChange({
                    page: 1,
                    start: parseDateTime(event.target.value),
                  })
                }
                type='datetime-local'
                value={dateTimeValue(search.start)}
              />
            </label>
            <label className='grid gap-1.5'>
              <span className='text-muted-foreground text-xs'>
                {t('upstreamTasks.filters.end')}
              </span>
              <Input
                aria-label={t('upstreamTasks.filters.end')}
                className='h-8'
                onChange={(event) =>
                  onChange({
                    end: parseDateTime(event.target.value),
                    page: 1,
                  })
                }
                type='datetime-local'
                value={dateTimeValue(search.end)}
              />
            </label>
          </div>
        </PopoverContent>
      </Popover>
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

function Breakdown({
  items,
  site = false,
  title,
}: {
  items: UpstreamTaskBreakdown[]
  site?: boolean
  title: string
}) {
  const { t } = useTranslation()
  return (
    <section className='grid gap-3'>
      <h3 className='font-semibold'>{title}</h3>
      {items.length === 0 ? (
        <p className='text-muted-foreground text-sm'>{t('common.none')}</p>
      ) : (
        <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
          {items.map((item) => (
            <article
              className='border-border grid gap-2 rounded-lg border p-3'
              key={`${item.site_id}:${item.dimension_id}`}
            >
              <div className='flex items-start justify-between gap-2'>
                <div>
                  <p className='font-medium'>{item.dimension_name || '-'}</p>
                  <code className='text-muted-foreground text-xs'>
                    {item.dimension_id || '-'}
                  </code>
                  {site && (
                    <p className='text-muted-foreground text-xs'>
                      {item.site_name} · {item.site_id}
                    </p>
                  )}
                </div>
                <DataStatusBadge status={item.data_status} />
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('upstreamTasks.breakdown.values', {
                  failure: item.failure,
                  running: item.running,
                  success: item.success,
                  total: item.total,
                })}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t('upstreamTasks.asOf', { time: timestamp(item.as_of) })}
              </p>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

export function UpstreamTasksPage({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<UpstreamTaskSearch>) => void
  search: UpstreamTaskSearch
  siteId?: string
}) {
  const { t } = useTranslation()
  const [initialJob, setInitialJob] = useState<StatisticsExportJobItem>()
  const validSiteId = siteId == null || isIdString(siteId)
  const currentParams = useMemo(() => params(search), [search])
  const overviewParams = useMemo(() => params(buildUpstreamTaskSearch({})), [])
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
      siteId && isIdString(siteId)
        ? listSiteUpstreamTasks(parseIdString(siteId), currentParams)
        : listUpstreamTasks(currentParams),
    queryKey:
      siteId && isIdString(siteId)
        ? upstreamTaskKeys.siteList(siteId, currentParams)
        : upstreamTaskKeys.globalList(currentParams),
  })
  const statisticsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? getSiteUpstreamTaskStatistics(parseIdString(siteId), overviewParams)
        : getUpstreamTaskStatistics(overviewParams),
    queryKey:
      siteId && isIdString(siteId)
        ? upstreamTaskKeys.siteStatistics(siteId, overviewParams)
        : upstreamTaskKeys.globalStatistics(overviewParams),
  })
  const exportMutation = useMutation({
    mutationFn: (format: StatisticsExportFormat) =>
      createStatisticsExport(
        buildUpstreamTaskExportRequest(
          format,
          search,
          siteId && isIdString(siteId) ? parseIdString(siteId) : undefined
        )
      ),
    onError: (error) =>
      toast.error(t(dynamicI18nKey('api', getApiErrorTranslationKey(error)))),
    onSuccess: (job) => {
      setInitialJob(job)
      onSearchChange({ exportId: job.id })
    },
  })
  const list = listQuery.data
  const statistics = statisticsQuery.data
  const columns = useMemo<ColumnDef<UpstreamTaskItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='min-w-44'>
            <code className='font-medium'>{row.original.task_id}</code>
            <span className='text-muted-foreground block text-xs'>
              {row.original.site_name} · {row.original.site_id}
            </span>
            <span className='text-muted-foreground block text-xs'>
              {t('upstreamTasks.remoteIdValue', {
                value: row.original.remote_id,
              })}
            </span>
          </div>
        ),
        header: t('upstreamTasks.identity'),
        id: 'identity',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-32 gap-1'>
            <TaskStatusBadge status={row.original.status} />
            <span className='text-xs'>{row.original.progress || '-'}</span>
          </div>
        ),
        header: t('upstreamTasks.statusProgress'),
        id: 'status',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-36 gap-1 text-xs'>
            <span>{row.original.platform || '-'}</span>
            <span>{row.original.action || '-'}</span>
            <span>{row.original.properties.model || '-'}</span>
            <span>{row.original.group || '-'}</span>
          </div>
        ),
        header: t('upstreamTasks.classification'),
        id: 'classification',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-36 gap-1 text-xs'>
            <span>
              {t('upstreamTasks.userValue', { value: row.original.user_id })}
            </span>
            <span>
              {t('upstreamTasks.channelValue', {
                value: row.original.channel_id,
              })}
            </span>
            <span>
              {t('upstreamTasks.quotaValue', { value: row.original.quota })}
            </span>
          </div>
        ),
        header: t('upstreamTasks.operationalValues'),
        id: 'values',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-48 gap-1 text-xs'>
            <span>
              {t('upstreamTasks.submitValue', {
                value: timestamp(row.original.submit_time),
              })}
            </span>
            <span>
              {t('upstreamTasks.startValue', {
                value: timestamp(row.original.start_time),
              })}
            </span>
            <span>
              {t('upstreamTasks.finishValue', {
                value: timestamp(row.original.finish_time),
              })}
            </span>
            <span>
              {t('upstreamTasks.seenValue', {
                first: timestamp(row.original.first_seen_at),
                last: timestamp(row.original.last_seen_at),
              })}
            </span>
          </div>
        ),
        header: t('upstreamTasks.timestamps'),
        id: 'timestamps',
      },
    ],
    [t]
  )
  const tabs = [
    {
      count: statistics?.summary.total,
      icon: Database01Icon,
      label: t('upstreamTasks.tabs.list'),
      value: 'list',
    },
    {
      count: statistics?.status_breakdown.length,
      icon: Alert02Icon,
      label: t('upstreamTasks.tabs.statuses'),
      value: 'statuses',
    },
    {
      count: statistics?.platform_breakdown.length,
      icon: Chart01Icon,
      label: t('upstreamTasks.tabs.platforms'),
      value: 'platforms',
    },
    {
      count: statistics?.action_breakdown.length,
      icon: Chart01Icon,
      label: t('upstreamTasks.tabs.actions'),
      value: 'actions',
    },
    {
      count: statistics?.model_breakdown.length,
      icon: Chart01Icon,
      label: t('upstreamTasks.tabs.models'),
      value: 'models',
    },
    {
      count: statistics?.site_breakdown.length,
      icon: Database01Icon,
      label: t('upstreamTasks.tabs.sites'),
      value: 'sites',
    },
  ] as const
  const breakdowns = {
    actions: statistics?.action_breakdown ?? [],
    models: statistics?.model_breakdown ?? [],
    platforms: statistics?.platform_breakdown ?? [],
    sites: statistics?.site_breakdown ?? [],
    statuses: statistics?.status_breakdown ?? [],
  }
  const breakdownTitles = {
    actions: t('upstreamTasks.breakdown.action'),
    models: t('upstreamTasks.breakdown.model'),
    platforms: t('upstreamTasks.breakdown.platform'),
    sites: t('upstreamTasks.breakdown.site'),
    statuses: t('upstreamTasks.breakdown.status'),
  }
  const purpose = {
    actions: {
      description: t('upstreamTasks.purpose.actionsDescription'),
      title: t('upstreamTasks.purpose.actionsTitle'),
    },
    list: {
      description: t('upstreamTasks.purpose.listDescription'),
      title: t('upstreamTasks.purpose.listTitle'),
    },
    models: {
      description: t('upstreamTasks.purpose.modelsDescription'),
      title: t('upstreamTasks.purpose.modelsTitle'),
    },
    platforms: {
      description: t('upstreamTasks.purpose.platformsDescription'),
      title: t('upstreamTasks.purpose.platformsTitle'),
    },
    sites: {
      description: t('upstreamTasks.purpose.sitesDescription'),
      title: t('upstreamTasks.purpose.sitesTitle'),
    },
    statuses: {
      description: t('upstreamTasks.purpose.statusesDescription'),
      title: t('upstreamTasks.purpose.statusesTitle'),
    },
  }[search.tab]
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
                {t('upstreamTasks.export', { format: format.toUpperCase() })}
              </Button>
            ))
          : undefined
      }
      description={
        siteId
          ? t('upstreamTasks.siteDescription', { id: siteId })
          : t('upstreamTasks.description')
      }
      fixedContent
      title={siteId ? t('upstreamTasks.siteTitle') : t('upstreamTasks.title')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('upstreamTasks.backToSite')}
          </DetailBackLink>
        )}
        {statistics && <MetricGrid metric={statistics.summary} />}
        <Tabs
          onValueChange={(tab) =>
            onSearchChange(
              changeUpstreamTaskTab(tab as UpstreamTaskSearch['tab'])
            )
          }
          value={search.tab}
        >
          <TabsList
            aria-label={t('upstreamTasks.tabs.label')}
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
        <section className='border-border bg-muted/30 flex items-start gap-3 rounded-xl border p-4'>
          <span className='bg-background text-muted-foreground ring-foreground/10 flex size-9 shrink-0 items-center justify-center rounded-lg ring-1'>
            <HugeiconsIcon
              icon={search.tab === 'list' ? Database01Icon : Chart01Icon}
              size={18}
              strokeWidth={2}
            />
          </span>
          <div className='min-w-0 flex-1'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <p className='font-medium'>{purpose.title}</p>
              <DataStatusBadge
                status={
                  search.tab === 'list'
                    ? (list?.data_status ?? 'pending')
                    : (statistics?.data_status ?? 'pending')
                }
              />
            </div>
            <p className='text-muted-foreground mt-1 text-sm'>
              {purpose.description}
            </p>
            <p className='text-muted-foreground mt-1 text-xs'>
              {search.tab === 'list' && list
                ? t('upstreamTasks.viewMeta', {
                    time: timestamp(list.as_of),
                    total: list.total,
                  })
                : t('upstreamTasks.statisticsScope')}
            </p>
          </div>
        </section>
        {search.tab === 'list' && (
          <Filters
            global={!siteId}
            onChange={onSearchChange}
            search={search}
            sites={sitesQuery.data?.items ?? []}
          />
        )}
        {search.tab === 'list' && (
          <DataTable
            ariaLabel={t('upstreamTasks.table')}
            columns={columns}
            data={list?.items ?? []}
            emptyDescription={t('upstreamTasks.emptyDescription')}
            emptyTitle={t('upstreamTasks.empty')}
            error={!validSiteId || listQuery.isError}
            fetching={listQuery.isFetching}
            loading={listQuery.isPending}
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={validSiteId ? () => void listQuery.refetch() : undefined}
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(item) => (
              <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
                <div className='flex items-start justify-between gap-2'>
                  <div className='min-w-0'>
                    <code className='block truncate font-medium'>
                      {item.task_id}
                    </code>
                    <span className='text-muted-foreground text-xs'>
                      {item.site_name} · {item.site_id}
                    </span>
                  </div>
                  <TaskStatusBadge status={item.status} />
                </div>
                <p className='text-sm'>{item.progress || '-'}</p>
                <dl className='grid grid-cols-2 gap-3 text-sm'>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('upstreamTasks.platform')}
                    </dt>
                    <dd>{item.platform || '-'}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('upstreamTasks.action')}
                    </dt>
                    <dd>{item.action || '-'}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('upstreamTasks.model')}
                    </dt>
                    <dd>{item.properties.model || '-'}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('upstreamTasks.quota')}
                    </dt>
                    <dd>{item.quota}</dd>
                  </div>
                </dl>
              </article>
            )}
            total={list?.total ?? 0}
          />
        )}
        {statisticsQuery.isError && !statistics && (
          <ErrorState
            className='min-h-40'
            onRetry={() => void statisticsQuery.refetch()}
            title={t('upstreamTasks.statisticsError')}
          />
        )}
        {statistics && search.tab !== 'list' && (
          <div className='min-h-0 flex-1 overflow-y-auto' tabIndex={0}>
            <Breakdown
              items={breakdowns[search.tab]}
              site={search.tab === 'sites'}
              title={breakdownTitles[search.tab]}
            />
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
