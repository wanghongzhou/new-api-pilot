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
import { ErrorState } from '@/components/error-state'
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
  const values = [
    [t('upstreamTasks.metric.total'), metric.total],
    [t('upstreamTasks.metric.queued'), metric.queued],
    [t('upstreamTasks.metric.running'), metric.running],
    [t('upstreamTasks.metric.success'), metric.success],
    [t('upstreamTasks.metric.failure'), metric.failure],
    [t('upstreamTasks.metric.successRate'), metric.success_rate],
    [t('upstreamTasks.metric.avgQueue'), metric.avg_queue_seconds],
    [t('upstreamTasks.metric.avgRun'), metric.avg_run_seconds],
    [t('upstreamTasks.metric.avgTotal'), metric.avg_total_seconds],
  ] as const
  return (
    <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-3 xl:grid-cols-9'>
      {values.map(([label, value]) => (
        <div
          className='border-border min-w-0 border-b p-3 xl:border-r'
          key={label}
        >
          <dt className='text-muted-foreground text-xs'>{label}</dt>
          <dd className='mt-1 text-lg font-semibold'>
            <MetricValue value={value} />
          </dd>
        </div>
      ))}
    </dl>
  )
}

function commaValues(value: string) {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

function Filters({
  global,
  onChange,
  search,
}: {
  global: boolean
  onChange: (changes: Partial<UpstreamTaskSearch>) => void
  search: UpstreamTaskSearch
}) {
  const { t } = useTranslation()
  const reset = buildUpstreamTaskSearch({ pageSize: search.pageSize })
  const textFilter = (
    key: 'actions' | 'groups' | 'models' | 'platforms',
    label: string
  ) => (
    <label className='grid gap-1 text-sm'>
      <span>{label}</span>
      <Input
        onChange={(event) =>
          onChange({ [key]: commaValues(event.target.value), page: 1 })
        }
        value={search[key].join(',')}
      />
    </label>
  )
  return (
    <FilterPanel
      description={t('upstreamTasks.filters.description')}
      hasActiveFilters={hasFilterChanges(search, reset, [
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
      ])}
      onReset={() => onChange(reset)}
      title={t('upstreamTasks.filters.title')}
    >
      <div className='grid min-w-0 flex-1 gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        {global && (
          <label className='grid gap-1 text-sm'>
            <span>{t('upstreamTasks.filters.siteIds')}</span>
            <Input
              inputMode='numeric'
              onChange={(event) =>
                onChange({
                  page: 1,
                  siteIds: commaValues(event.target.value)
                    .filter(isIdString)
                    .map(parseIdString),
                })
              }
              value={search.siteIds.join(',')}
            />
          </label>
        )}
        <label className='grid gap-1 text-sm'>
          <span>{t('upstreamTasks.filters.remoteId')}</span>
          <Input
            inputMode='numeric'
            onChange={(event) => {
              const value = event.target.value
              onChange({
                page: 1,
                remoteId: isIdString(value) ? parseIdString(value) : undefined,
              })
            }}
            value={search.remoteId ?? ''}
          />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('upstreamTasks.filters.taskId')}</span>
          <Input
            onChange={(event) =>
              onChange({ page: 1, taskId: event.target.value })
            }
            value={search.taskId}
          />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('upstreamTasks.filters.remoteUserId')}</span>
          <Input
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
        <label className='grid gap-1 text-sm'>
          <span>{t('upstreamTasks.filters.remoteChannelId')}</span>
          <Input
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
        {textFilter('platforms', t('upstreamTasks.filters.platforms'))}
        {textFilter('groups', t('upstreamTasks.filters.groups'))}
        {textFilter('actions', t('upstreamTasks.filters.actions'))}
        {textFilter('models', t('upstreamTasks.filters.models'))}
        <label className='grid gap-1 text-sm'>
          <span>{t('upstreamTasks.filters.start')}</span>
          <Input
            onChange={(event) =>
              onChange({ page: 1, start: parseDateTime(event.target.value) })
            }
            type='datetime-local'
            value={dateTimeValue(search.start)}
          />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('upstreamTasks.filters.end')}</span>
          <Input
            onChange={(event) =>
              onChange({ end: parseDateTime(event.target.value), page: 1 })
            }
            type='datetime-local'
            value={dateTimeValue(search.end)}
          />
        </label>
      </div>
      <fieldset className='grid gap-2'>
        <legend className='text-sm'>
          {t('upstreamTasks.filters.statuses')}
        </legend>
        <div className='flex flex-wrap gap-2'>
          {upstreamTaskStatuses.map((status) => {
            const active = search.statuses.includes(status)
            return (
              <Button
                aria-pressed={active}
                key={status}
                onClick={() =>
                  onChange({
                    page: 1,
                    statuses: active
                      ? search.statuses.filter((item) => item !== status)
                      : [...search.statuses, status],
                  })
                }
                size='sm'
                type='button'
                variant={active ? 'secondary' : 'outline'}
              >
                {statusText(t, status)}
              </Button>
            )
          })}
        </div>
      </fieldset>
    </FilterPanel>
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
        <div className='grid gap-2'>
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
  const listQuery = useQuery({
    enabled: validSiteId,
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
        ? getSiteUpstreamTaskStatistics(parseIdString(siteId), currentParams)
        : getUpstreamTaskStatistics(currentParams),
    queryKey:
      siteId && isIdString(siteId)
        ? upstreamTaskKeys.siteStatistics(siteId, currentParams)
        : upstreamTaskKeys.globalStatistics(currentParams),
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
          {t('upstreamTasks.export', { format: format.toUpperCase() })}
        </Button>
      ))}
      description={
        siteId
          ? t('upstreamTasks.siteDescription', { id: siteId })
          : t('upstreamTasks.description')
      }
      title={siteId ? t('upstreamTasks.siteTitle') : t('upstreamTasks.title')}
    >
      <div className='grid min-w-0 gap-6'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('upstreamTasks.backToSite')}
          </DetailBackLink>
        )}
        <section
          className='border-primary/30 bg-primary/5 rounded-lg border p-4'
          role='note'
        >
          <p className='font-medium'>{t('upstreamTasks.boundary.title')}</p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('upstreamTasks.boundary.description')}
          </p>
        </section>
        <Filters global={!siteId} onChange={onSearchChange} search={search} />
        <div className='grid gap-3 sm:grid-cols-2'>
          {list && (
            <section
              className='border-border flex items-center gap-2 rounded-lg border p-3'
              role='status'
            >
              <span className='text-sm font-medium'>
                {t('upstreamTasks.listStatus')}
              </span>
              <DataStatusBadge status={list.data_status} />
              <span className='text-muted-foreground text-xs'>
                {timestamp(list.as_of)}
              </span>
            </section>
          )}
          {statistics && (
            <section
              className='border-border flex items-center gap-2 rounded-lg border p-3'
              role='status'
            >
              <span className='text-sm font-medium'>
                {t('upstreamTasks.statisticsStatus')}
              </span>
              <DataStatusBadge status={statistics.data_status} />
            </section>
          )}
        </div>
        {statistics && <MetricGrid metric={statistics.summary} />}
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
          onPageSizeChange={(pageSize) => onSearchChange({ page: 1, pageSize })}
          onRetry={() => void listQuery.refetch()}
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
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('upstreamTasks.user')}
                  </dt>
                  <dd>{item.user_id}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('upstreamTasks.channel')}
                  </dt>
                  <dd>{item.channel_id}</dd>
                </div>
              </dl>
            </article>
          )}
          total={list?.total ?? 0}
        />
        {statisticsQuery.isError && !statistics && (
          <ErrorState
            className='min-h-40'
            onRetry={() => void statisticsQuery.refetch()}
            title={t('upstreamTasks.statisticsError')}
          />
        )}
        {statistics && (
          <div className='grid gap-6 xl:grid-cols-2'>
            <Breakdown
              items={statistics.status_breakdown}
              title={t('upstreamTasks.breakdown.status')}
            />
            <Breakdown
              items={statistics.platform_breakdown}
              title={t('upstreamTasks.breakdown.platform')}
            />
            <Breakdown
              items={statistics.action_breakdown}
              title={t('upstreamTasks.breakdown.action')}
            />
            <Breakdown
              items={statistics.model_breakdown}
              title={t('upstreamTasks.breakdown.model')}
            />
            <div className='xl:col-span-2'>
              <Breakdown
                items={statistics.site_breakdown}
                site
                title={t('upstreamTasks.breakdown.site')}
              />
            </div>
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
