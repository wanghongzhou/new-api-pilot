import { ArrowLeft01Icon, Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { LineChart } from '@visactor/react-vchart'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Input } from '@/components/ui/input'
import { SelectControl as Select } from '@/components/ui/select-control'
import { getSettings } from '@/features/settings/api'
import { getMinuteRetentionDays } from '@/features/settings/contract'
import { settingsKeys } from '@/features/settings/query-keys'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { isIdString, parseIdString } from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'

import { getSite, getSiteResource, listSiteInstances } from '../api'
import { siteKeys } from '../query-keys'
import type {
  ResourcePoint,
  SiteHealthStatus,
  SiteInstanceItem,
  SiteResourceQuery,
} from '../types'

export interface SiteStatusSearch {
  aggregation: 'avg' | 'last' | 'max'
  end: number
  granularity: 'day' | 'hour' | 'minute'
  metric: 'cpu' | 'disk' | 'memory'
  nodeName?: string
  start: number
}

interface SiteInstancesPageProps {
  onSearchChange: (changes: Partial<SiteStatusSearch>) => void
  search: SiteStatusSearch
  siteId: string
}

function statusVariant(
  status: SiteInstanceItem['current_status']
): 'destructive' | 'neutral' | 'success' | 'warning' {
  switch (status) {
    case 'online':
      return 'success'
    case 'stale':
      return 'warning'
    case 'offline':
      return 'destructive'
    case 'unknown':
      return 'neutral'
  }
}

function healthVariant(
  status: SiteHealthStatus
): 'destructive' | 'neutral' | 'success' | 'warning' {
  switch (status) {
    case 'ok':
      return 'success'
    case 'warning':
      return 'warning'
    case 'critical':
      return 'destructive'
    case 'unavailable':
      return 'neutral'
  }
}

function InstanceStatusBadge({
  status,
}: {
  status: SiteInstanceItem['current_status']
}) {
  const { t } = useTranslation()
  return (
    <Badge variant={statusVariant(status)}>
      {t(dynamicI18nKey('site', `instance.status.${status}`))}
    </Badge>
  )
}

function PercentValue({ value }: { value: number | null }) {
  return <span>{`${(value ?? 0).toFixed(1)}%`}</span>
}

function TimestampValue({ timestamp }: { timestamp: number | null }) {
  if (timestamp == null) return <span>-</span>
  return <span>{fromUnixSeconds(timestamp).format('YYYY-MM-DD HH:mm:ss')}</span>
}

function maximum(
  instances: SiteInstanceItem[],
  value: (instance: SiteInstanceItem) => number | null
): number | null {
  const values = instances
    .map(value)
    .filter((item): item is number => item != null)
  return values.length === 0 ? null : Math.max(...values)
}

function SummaryCell({
  children,
  label,
}: {
  children: ReactNode
  label: string
}) {
  return (
    <div className='min-w-0 px-4 py-3'>
      <dt className='text-muted-foreground text-xs'>{label}</dt>
      <dd className='mt-1 text-lg font-semibold'>{children}</dd>
    </div>
  )
}

function SummarySkeleton() {
  return (
    <span
      aria-hidden='true'
      className='bg-muted block h-6 w-16 animate-pulse rounded-sm'
    />
  )
}

function InstanceCard({ instance }: { instance: SiteInstanceItem }) {
  const { t } = useTranslation()
  return (
    <article className='border-border bg-card grid min-w-0 gap-3 rounded-lg border p-4'>
      <div className='flex min-w-0 items-start justify-between gap-3'>
        <div className='min-w-0'>
          <h3 className='truncate font-medium' title={instance.node_name}>
            {instance.node_name}
          </h3>
          <p className='text-muted-foreground truncate text-xs'>
            {instance.hostname || '-'}
          </p>
        </div>
        <InstanceStatusBadge status={instance.current_status} />
      </div>
      <dl className='grid grid-cols-3 gap-3 text-sm'>
        <div>
          <dt className='text-muted-foreground text-xs'>{t('metric.cpu')}</dt>
          <dd>
            <PercentValue value={instance.cpu_percent} />
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('metric.memory')}
          </dt>
          <dd>
            <PercentValue value={instance.memory_percent} />
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>{t('metric.disk')}</dt>
          <dd>
            <PercentValue value={instance.disk_used_percent} />
          </dd>
        </div>
      </dl>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <DataStatusBadge status={instance.data_status} />
        <DataFreshness
          labelKey='instance.sampledAt'
          timestamp={instance.sampled_at}
        />
      </div>
      <details className='text-xs'>
        <summary className='text-muted-foreground cursor-pointer py-1'>
          {t('instance.technicalDetails')}
        </summary>
        <dl className='mt-2 grid grid-cols-2 gap-2'>
          <div>
            <dt className='text-muted-foreground'>
              {t('instance.upstreamStatus')}
            </dt>
            <dd>
              {t(
                dynamicI18nKey(
                  'site',
                  `instance.upstream.${instance.upstream_status}`
                )
              )}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>{t('instance.runtime')}</dt>
            <dd>{instance.runtime_version || '-'}</dd>
          </div>
        </dl>
      </details>
    </article>
  )
}

function resourceValue(
  point: ResourcePoint,
  metric: SiteStatusSearch['metric'],
  aggregation: SiteStatusSearch['aggregation']
): number | null {
  if (metric === 'cpu') {
    return aggregation === 'avg' ? point.cpu_avg_percent : point.cpu_max_percent
  }
  if (metric === 'memory') {
    return aggregation === 'avg'
      ? point.memory_avg_percent
      : point.memory_max_percent
  }
  return aggregation === 'last'
    ? point.disk_last_used_percent
    : point.disk_max_used_percent
}

function formatBucket(
  timestamp: number,
  granularity: SiteStatusSearch['granularity']
) {
  const value = fromUnixSeconds(timestamp)
  if (granularity === 'minute') return value.format('MM-DD HH:mm')
  if (granularity === 'hour') return value.format('MM-DD HH:00')
  return value.format('YYYY-MM-DD')
}

function ResourceTrend({
  error,
  loading,
  points,
  search,
}: {
  error: boolean
  loading: boolean
  points: ResourcePoint[]
  search: SiteStatusSearch
}) {
  const { t } = useTranslation()
  const data = useMemo(
    () =>
      points.map((point) => ({
        health: t(dynamicI18nKey('site', `site.health.${point.health_status}`)),
        time: formatBucket(point.bucket_start, search.granularity),
        value: resourceValue(point, search.metric, search.aggregation),
      })),
    [points, search.aggregation, search.granularity, search.metric, t]
  )

  if (loading) {
    return (
      <div
        aria-label={t('resource.loading')}
        className='border-border bg-muted/35 flex h-80 items-center justify-center rounded-lg border'
        role='status'
      >
        <span className='text-muted-foreground text-sm'>
          {t('resource.loading')}
        </span>
      </div>
    )
  }
  if (error) {
    return (
      <div className='border-destructive/30 bg-destructive/5 rounded-lg border p-5'>
        <p className='text-destructive text-sm'>{t('resource.loadError')}</p>
      </div>
    )
  }
  if (data.length === 0) {
    return (
      <div className='border-border text-muted-foreground rounded-lg border px-4 py-12 text-center text-sm'>
        {t('resource.empty')}
      </div>
    )
  }

  return (
    <div className='border-border h-80 min-w-0 rounded-lg border p-2'>
      <LineChart
        animation={false}
        axes={[
          {
            orient: 'left',
            title: { text: t('resource.percentAxis'), visible: true },
          },
          { label: { autoHide: true, autoRotate: true }, orient: 'bottom' },
        ]}
        data={{ id: 'resource-trend', values: data }}
        height={300}
        invalidType='break'
        point={{ state: { hover: { size: 7 } }, style: { size: 4 } }}
        tooltip={{ visible: true }}
        xField='time'
        yField='value'
      />
      <table className='sr-only'>
        <caption>{t('resource.accessibleTable')}</caption>
        <thead>
          <tr>
            <th>{t('resource.time')}</th>
            <th>{t('resource.value')}</th>
          </tr>
        </thead>
        <tbody>
          {data.map((point) => (
            <tr key={point.time}>
              <td>{point.time}</td>
              <td>
                {point.value == null
                  ? t('data.unavailableValue')
                  : `${point.value.toFixed(1)}%`}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export function SiteInstancesPage({
  onSearchChange,
  search,
  siteId,
}: SiteInstancesPageProps) {
  const { t } = useTranslation()
  const validSiteId = isIdString(siteId)
  const invalidRange = search.start >= search.end
  const settingsQuery = useQuery({
    queryFn: getSettings,
    queryKey: settingsKeys.all,
    staleTime: 30_000,
  })
  const minuteRetentionDays = getMinuteRetentionDays(settingsQuery.data)
  const minuteRetentionUnavailable =
    search.granularity === 'minute' &&
    (settingsQuery.isPending ||
      settingsQuery.isError ||
      minuteRetentionDays == null)
  const minuteRangeTooLong =
    search.granularity === 'minute' &&
    minuteRetentionDays != null &&
    search.end - search.start > minuteRetentionDays * 24 * 60 * 60
  const resourceParams = useMemo<SiteResourceQuery>(
    () => ({
      end_timestamp: search.end,
      granularity: search.granularity,
      node_name: search.nodeName || undefined,
      start_timestamp: search.start,
    }),
    [search.end, search.granularity, search.nodeName, search.start]
  )
  const detailQuery = useQuery({
    enabled: validSiteId,
    queryFn: () => getSite(parseIdString(siteId)),
    queryKey: siteKeys.detail(siteId),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })
  const instancesQuery = useQuery({
    enabled: validSiteId,
    queryFn: () => listSiteInstances(parseIdString(siteId)),
    queryKey: siteKeys.instances(siteId),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })
  const resourceQuery = useQuery({
    enabled:
      validSiteId &&
      !invalidRange &&
      !minuteRetentionUnavailable &&
      !minuteRangeTooLong,
    queryFn: () => getSiteResource(parseIdString(siteId), resourceParams),
    queryKey: siteKeys.status(siteId, resourceParams),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })
  const instances = instancesQuery.data ?? []
  const onlineCount = instances.filter(
    (instance) => instance.current_status === 'online'
  ).length

  const columns = useMemo<ColumnDef<SiteInstanceItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='min-w-40'>
            <div className='flex items-center gap-2'>
              <span className='font-medium'>{row.original.node_name}</span>
              {row.original.is_master && (
                <Badge variant='primary'>{t('instance.master')}</Badge>
              )}
            </div>
            <p className='text-muted-foreground text-xs'>
              {row.original.hostname}
            </p>
          </div>
        ),
        header: t('instance.node'),
        id: 'node',
      },
      {
        cell: ({ row }) => (
          <InstanceStatusBadge status={row.original.current_status} />
        ),
        header: t('common.status'),
        id: 'status',
      },
      {
        cell: ({ row }) => (
          <div className='text-sm whitespace-nowrap'>
            <span>{row.original.runtime_version}</span>
            <p className='text-muted-foreground text-xs'>
              {row.original.goos}/{row.original.goarch}
            </p>
          </div>
        ),
        header: t('instance.runtime'),
        id: 'runtime',
      },
      {
        cell: ({ row }) => <PercentValue value={row.original.cpu_percent} />,
        header: t('metric.cpu'),
        id: 'cpu',
      },
      {
        cell: ({ row }) => <PercentValue value={row.original.memory_percent} />,
        header: t('metric.memory'),
        id: 'memory',
      },
      {
        cell: ({ row }) => (
          <PercentValue value={row.original.disk_used_percent} />
        ),
        header: t('metric.disk'),
        id: 'disk',
      },
      {
        cell: ({ row }) => (
          <TimestampValue timestamp={row.original.started_at} />
        ),
        header: t('instance.startedAt'),
        id: 'startedAt',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-44 gap-1'>
            <TimestampValue timestamp={row.original.last_seen_at} />
            <DataFreshness
              labelKey='instance.sampledAt'
              timestamp={row.original.sampled_at}
            />
          </div>
        ),
        header: t('instance.lastSeenAt'),
        id: 'lastSeenAt',
      },
      {
        cell: ({ row }) => (
          <DataStatusBadge status={row.original.data_status} />
        ),
        header: t('instance.dataStatus'),
        id: 'dataStatus',
      },
    ],
    [t]
  )

  const changeMetric = (metric: SiteStatusSearch['metric']) => {
    let aggregation = search.aggregation
    if (metric === 'disk' && aggregation === 'avg') aggregation = 'last'
    if (metric !== 'disk' && aggregation === 'last') aggregation = 'max'
    onSearchChange({ aggregation, metric })
  }
  const parseDateTime = (value: string): number | null => {
    const parsed = dayjs.tz(value, 'YYYY-MM-DDTHH:mm', BEIJING_TIMEZONE)
    return parsed.isValid() ? parsed.unix() : null
  }

  const detail = detailQuery.data
  const healthStatus =
    resourceQuery.data?.summary?.health_status ??
    detail?.health_status ??
    'unavailable'
  const retentionLoading =
    search.granularity === 'minute' && settingsQuery.isPending
  let rangeError: string | null = null
  if (invalidRange) rangeError = t('resource.invalidRange')
  else if (retentionLoading) {
    rangeError = t('resource.retentionLoading')
  } else if (
    search.granularity === 'minute' &&
    (settingsQuery.isError || minuteRetentionDays == null)
  ) {
    rangeError = t('resource.retentionLoadError')
  } else if (minuteRangeTooLong) {
    rangeError = t('resource.minuteRangeLimit', { days: minuteRetentionDays })
  }

  return (
    <SectionPageLayout
      actions={
        <Button
          disabled={
            instancesQuery.isFetching ||
            resourceQuery.isFetching ||
            (search.granularity === 'minute' && settingsQuery.isFetching)
          }
          onClick={() => {
            void instancesQuery.refetch()
            if (search.granularity === 'minute') void settingsQuery.refetch()
            if (!rangeError) void resourceQuery.refetch()
          }}
          variant='outline'
        >
          <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
          {t('common.refresh')}
        </Button>
      }
      description={detail?.base_url ?? t('instance.description')}
      title={detail?.name ?? t('instance.title')}
    >
      <div className='grid min-w-0 gap-8'>
        <DetailBackLink
          render={<Link params={{ siteId }} to='/sites/$siteId' />}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          {t('instance.backToSite')}
        </DetailBackLink>

        {!validSiteId && (
          <section className='border-destructive/30 bg-destructive/5 rounded-lg border p-5'>
            <h2 className='font-medium'>{t('instance.invalidSite')}</h2>
          </section>
        )}

        <section
          aria-labelledby='instance-summary-title'
          className='grid gap-3'
        >
          <h2 className='text-lg font-semibold' id='instance-summary-title'>
            {t('instance.summary')}
          </h2>
          <dl className='border-border [&>div]:border-border grid overflow-hidden rounded-lg border sm:grid-cols-3 xl:grid-cols-6 [&>div]:border-b sm:[&>div]:border-r xl:[&>div:nth-child(6n)]:border-r-0'>
            <SummaryCell label={t('instance.total')}>
              {instancesQuery.isPending ? (
                <SummarySkeleton />
              ) : (
                instances.length
              )}
            </SummaryCell>
            <SummaryCell label={t('instance.online')}>
              {instancesQuery.isPending ? <SummarySkeleton /> : onlineCount}
            </SummaryCell>
            <SummaryCell label={t('metric.cpuMax')}>
              {instancesQuery.isPending ? (
                <SummarySkeleton />
              ) : (
                <PercentValue
                  value={maximum(instances, (item) => item.cpu_percent)}
                />
              )}
            </SummaryCell>
            <SummaryCell label={t('metric.memoryMax')}>
              {instancesQuery.isPending ? (
                <SummarySkeleton />
              ) : (
                <PercentValue
                  value={maximum(instances, (item) => item.memory_percent)}
                />
              )}
            </SummaryCell>
            <SummaryCell label={t('metric.diskMax')}>
              {instancesQuery.isPending ? (
                <SummarySkeleton />
              ) : (
                <PercentValue
                  value={maximum(instances, (item) => item.disk_used_percent)}
                />
              )}
            </SummaryCell>
            <SummaryCell label={t('site.healthStatus')}>
              {instancesQuery.isPending ? (
                <SummarySkeleton />
              ) : (
                <Badge variant={healthVariant(healthStatus)}>
                  {t(dynamicI18nKey('site', `site.health.${healthStatus}`))}
                </Badge>
              )}
            </SummaryCell>
          </dl>
        </section>

        <section
          aria-labelledby='instances-current-title'
          className='grid gap-3'
        >
          <div>
            <h2 className='text-lg font-semibold' id='instances-current-title'>
              {t('instance.currentTitle')}
            </h2>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('instance.currentDescription')}
            </p>
          </div>
          <DataTable
            ariaLabel={t('instance.table')}
            columns={columns}
            data={instances}
            emptyDescription={t('instance.emptyDescription')}
            emptyTitle={t('instance.empty')}
            error={!validSiteId || instancesQuery.isError}
            fetching={instancesQuery.isFetching}
            loading={instancesQuery.isPending && validSiteId}
            onRetry={() => void instancesQuery.refetch()}
            renderMobileCard={(instance) => (
              <InstanceCard instance={instance} />
            )}
          />
        </section>

        <section aria-labelledby='resource-trend-title' className='grid gap-4'>
          <div>
            <h2 className='text-lg font-semibold' id='resource-trend-title'>
              {t('resource.title')}
            </h2>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('resource.description')}
            </p>
          </div>
          <div className='grid gap-3 border-y py-4 lg:grid-cols-2 xl:grid-cols-4'>
            <label className='grid gap-1 text-sm'>
              <span>{t('resource.instance')}</span>
              <Select
                onChange={(event) =>
                  onSearchChange({ nodeName: event.target.value || undefined })
                }
                value={search.nodeName ?? ''}
              >
                <option value=''>{t('resource.allInstances')}</option>
                {instances.map((instance) => (
                  <option key={instance.node_name} value={instance.node_name}>
                    {instance.node_name}
                  </option>
                ))}
              </Select>
            </label>
            <label className='grid gap-1 text-sm'>
              <span>{t('resource.granularity')}</span>
              <Select
                onChange={(event) =>
                  onSearchChange({
                    granularity: event.target
                      .value as SiteStatusSearch['granularity'],
                  })
                }
                value={search.granularity}
              >
                <option value='minute'>
                  {t('resource.granularity.minute')}
                </option>
                <option value='hour'>{t('resource.granularity.hour')}</option>
                <option value='day'>{t('resource.granularity.day')}</option>
              </Select>
            </label>
            <label className='grid gap-1 text-sm'>
              <span>{t('resource.start')}</span>
              <Input
                max={fromUnixSeconds(search.end).format('YYYY-MM-DDTHH:mm')}
                onChange={(event) => {
                  const start = parseDateTime(event.target.value)
                  if (start != null) onSearchChange({ start })
                }}
                type='datetime-local'
                value={fromUnixSeconds(search.start).format('YYYY-MM-DDTHH:mm')}
              />
            </label>
            <label className='grid gap-1 text-sm'>
              <span>{t('resource.end')}</span>
              <Input
                min={fromUnixSeconds(search.start).format('YYYY-MM-DDTHH:mm')}
                onChange={(event) => {
                  const end = parseDateTime(event.target.value)
                  if (end != null) onSearchChange({ end })
                }}
                type='datetime-local'
                value={fromUnixSeconds(search.end).format('YYYY-MM-DDTHH:mm')}
              />
            </label>
          </div>
          <div className='flex flex-wrap items-center gap-3'>
            <div
              aria-label={t('resource.metric')}
              className='border-border flex flex-wrap rounded-md border p-0.5'
              role='group'
            >
              {(['cpu', 'memory', 'disk'] as const).map((metric) => (
                <Button
                  aria-pressed={search.metric === metric}
                  key={metric}
                  onClick={() => changeMetric(metric)}
                  size='sm'
                  variant={search.metric === metric ? 'secondary' : 'ghost'}
                >
                  {t(dynamicI18nKey('site', `metric.${metric}`))}
                </Button>
              ))}
            </div>
            <div
              aria-label={t('resource.aggregation')}
              className='border-border flex flex-wrap rounded-md border p-0.5'
              role='group'
            >
              {(search.metric === 'disk'
                ? (['max', 'last'] as const)
                : (['max', 'avg'] as const)
              ).map((aggregation) => (
                <Button
                  aria-pressed={search.aggregation === aggregation}
                  key={aggregation}
                  onClick={() => onSearchChange({ aggregation })}
                  size='sm'
                  variant={
                    search.aggregation === aggregation ? 'secondary' : 'ghost'
                  }
                >
                  {t(
                    dynamicI18nKey(
                      'site',
                      `resource.aggregation.${aggregation}`
                    )
                  )}
                </Button>
              ))}
            </div>
            {resourceQuery.data?.summary && (
              <DataStatusBadge
                status={resourceQuery.data.summary.data_status}
              />
            )}
          </div>
          {rangeError && (
            <p
              className={
                retentionLoading
                  ? 'text-muted-foreground text-sm'
                  : 'text-destructive text-sm'
              }
              role={retentionLoading ? 'status' : 'alert'}
            >
              {rangeError}
            </p>
          )}
          <ResourceTrend
            error={
              resourceQuery.isError ||
              (Boolean(rangeError) && !retentionLoading)
            }
            loading={
              retentionLoading || (resourceQuery.isPending && !rangeError)
            }
            points={resourceQuery.data?.trend ?? []}
            search={search}
          />
        </section>
      </div>
    </SectionPageLayout>
  )
}
