import {
  ArrowLeft01Icon,
  Chart01Icon,
  FileExportIcon,
  Refresh01Icon,
  ServerStack01Icon,
  UserGroupIcon,
  ViewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { BackfillProgress } from '@/components/data/backfill-progress'
import { CompletenessAlert } from '@/components/data/completeness-alert'
import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { QuotaAmount } from '@/components/data/quota-amount'
import { SiteStatusBadges } from '@/components/data/site-status-badges'
import { ErrorState } from '@/components/error-state'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { buildChannelInventorySearch } from '@/features/channel-inventory/search'
import { buildFinancialOperationsSearch } from '@/features/financial-operations/search'
import { buildLogSearch } from '@/features/logs/search'
import { buildModelCatalogSearch } from '@/features/model-catalog/search'
import { buildPerformanceHistorySearch } from '@/features/performance-history/search'
import { buildPricingGroupSearch } from '@/features/pricing-groups/search'
import { buildRankingSearch } from '@/features/rankings/search'
import { EntityStatistics } from '@/features/statistics/components/entity-statistics'
import { buildStatisticsSearch } from '@/features/statistics/search'
import type {
  EntityStatisticsParams,
  StatisticsSearch,
} from '@/features/statistics/types'
import { buildSubscriptionPlanSearch } from '@/features/subscription-plans/search'
import { buildSystemTaskSearch } from '@/features/system-tasks/search'
import { buildUpstreamTaskSearch } from '@/features/upstream-tasks/search'
import { buildUserInventorySearch } from '@/features/user-inventory/search'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { isIdString, parseIdString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import {
  formatDisplayValue,
  formatNumericDisplayValue,
} from '@/lib/display-value'
import { useAuthStore } from '@/stores/auth-store'

import {
  getSite,
  getSitePerformance,
  getSiteStatistics,
  listSiteInstances,
} from '../api'
import { siteKeys } from '../query-keys'
import type {
  CollectionRunItem,
  CollectionRunWindowItem,
  CollectionTaskType,
  SiteDetail,
  SiteInstanceItem,
  SitePerformanceSummary,
} from '../types'
import { CollectionRunsPanel } from './collection-runs-panel'
import { SiteActions, type SiteAction } from './site-actions'
import { SiteDialogs, type SiteDialogState } from './site-dialogs'

export interface SiteDetailSearch {
  runId?: string
  runPage: number
  runStatus?: CollectionRunItem['status']
  runTaskType?: CollectionTaskType
  windowPage: number
  windowStatus?: CollectionRunWindowItem['status']
}

interface SiteDetailPageProps {
  onDeleted: () => void
  onSearchChange: (changes: Partial<SiteDetailSearch>) => void
  search: SiteDetailSearch
  siteId: string
}

function TimestampValue({ timestamp }: { timestamp: number | null }) {
  if (timestamp == null) return <span>-</span>
  return <span>{fromUnixSeconds(timestamp).format('YYYY-MM-DD HH:mm:ss')}</span>
}

function PercentValue({ value }: { value: number | null }) {
  return <span>{`${(value ?? 0).toFixed(1)}%`}</span>
}

function InstanceStatusBadge({
  status,
}: {
  status: SiteInstanceItem['current_status']
}) {
  let variant: 'destructive' | 'neutral' | 'success' | 'warning' = 'neutral'
  if (status === 'online') variant = 'success'
  else if (status === 'offline') variant = 'destructive'
  else if (status === 'stale') variant = 'warning'
  const { t } = useTranslation()
  return (
    <Badge variant={variant}>
      {t(dynamicI18nKey('site', `instance.status.${status}`))}
    </Badge>
  )
}

function MetricCell({
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

function DetailSummary({ site }: { site: SiteDetail }) {
  const { t } = useTranslation()
  return (
    <section aria-labelledby='site-summary-title' className='grid gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <h2 className='text-lg font-semibold' id='site-summary-title'>
          {t('site.detail.overview')}
        </h2>
        <DataFreshness
          expired={site.realtime.expired}
          labelKey='site.currentUpdatedAt'
          timestamp={site.realtime.updated_at}
        />
      </div>
      <dl className='border-border [&>div]:border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 lg:grid-cols-4 [&>div]:border-b sm:[&>div]:border-r lg:[&>div:nth-child(4n)]:border-r-0'>
        <MetricCell label={t('metric.rpm')}>
          <MetricValue nullLabel='0' value={site.realtime.rpm} />
        </MetricCell>
        <MetricCell label={t('metric.tpm')}>
          <MetricValue nullLabel='0' value={site.realtime.tpm} />
        </MetricCell>
        <MetricCell label={t('site.todayRequests')}>
          <MetricValue nullLabel='0' value={site.today.request_count} />
        </MetricCell>
        <MetricCell label={t('site.activeUsers')}>
          <MetricValue nullLabel='0' value={site.today.active_users} />
        </MetricCell>
        <MetricCell label={t('site.todayQuota')}>
          <QuotaAmount
            nullLabel='0'
            quota={site.today.quota}
            rate={site.rate}
          />
        </MetricCell>
        <MetricCell label={t('metric.token')}>
          <MetricValue nullLabel='0' value={site.today.token_used} />
        </MetricCell>
        <MetricCell label={t('metric.cpu')}>
          <PercentValue value={site.resource.cpu_max_percent} />
        </MetricCell>
        <MetricCell label={t('metric.memory')}>
          <PercentValue value={site.resource.memory_max_percent} />
        </MetricCell>
        <MetricCell label={t('metric.disk')}>
          <PercentValue value={site.resource.disk_max_used_percent} />
        </MetricCell>
        <MetricCell label={t('site.completeness')}>
          {(site.completeness_rate * 100).toFixed(1)}%
        </MetricCell>
      </dl>
      <div className='flex flex-wrap items-center justify-between gap-3 border-b pb-4'>
        <div className='flex items-center gap-2'>
          <span className='text-muted-foreground text-sm'>
            {t('site.todayDataStatus')}
          </span>
          <DataStatusBadge status={site.today.data_status} />
        </div>
        <DataFreshness
          labelKey='site.businessAsOf'
          timestamp={site.today.as_of}
        />
      </div>
    </section>
  )
}

const performanceRanges = [24, 168, 720] as const
const performanceRangeLabels = {
  24: 'site.performance.range.24h',
  168: 'site.performance.range.7d',
  720: 'site.performance.range.30d',
} as const

function PerformanceHealth({
  error,
  onRangeChange,
  pending,
  performance,
  range,
}: {
  error: boolean
  onRangeChange: (hours: (typeof performanceRanges)[number]) => void
  pending: boolean
  performance: SitePerformanceSummary | undefined
  range: (typeof performanceRanges)[number]
}) {
  const { t } = useTranslation()
  let content: ReactNode
  if (pending && !performance) {
    content = (
      <div className='grid grid-cols-2 gap-px overflow-hidden rounded-lg border sm:grid-cols-4'>
        {Array.from({ length: 4 }, (_, index) => (
          <div className='bg-muted h-20 animate-pulse' key={index} />
        ))}
      </div>
    )
  } else if (error && !performance) {
    content = (
      <p className='text-destructive text-sm'>
        {t('site.performance.loadError')}
      </p>
    )
  } else {
    content = (
      <>
        <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 lg:grid-cols-4'>
          <MetricCell label={t('site.performance.requestCount')}>
            <MetricValue
              nullLabel='0'
              value={performance?.request_count ?? null}
            />
          </MetricCell>
          <MetricCell label={t('site.performance.successRate')}>
            <PercentValue value={performance?.success_rate ?? null} />
          </MetricCell>
          <MetricCell label={t('site.performance.avgLatency')}>
            <span>
              {t('site.performance.latencyValue', {
                value: (performance?.avg_latency_ms ?? 0).toFixed(0),
              })}
            </span>
          </MetricCell>
          <MetricCell label={t('site.performance.avgTps')}>
            <span>
              {t('site.performance.tpsValue', {
                value: (performance?.avg_tps ?? 0).toFixed(1),
              })}
            </span>
          </MetricCell>
        </dl>
        <div className='border-t pt-3'>
          <h3 className='text-sm font-medium'>
            {t('site.performance.models')}
          </h3>
          {(performance?.models.length ?? 0) === 0 ? (
            <p className='text-muted-foreground mt-2 text-sm'>-</p>
          ) : (
            <dl className='mt-2 grid gap-x-6 gap-y-2 sm:grid-cols-2 lg:grid-cols-3'>
              {performance?.models.map((model) => (
                <div
                  className='flex min-w-0 items-center justify-between gap-3'
                  key={model.model_name}
                >
                  <dt className='truncate text-sm' title={model.model_name}>
                    {model.model_name}
                  </dt>
                  <dd className='text-sm font-medium'>
                    <PercentValue value={model.success_rate} />
                  </dd>
                </div>
              ))}
            </dl>
          )}
        </div>
      </>
    )
  }

  return (
    <section aria-labelledby='site-performance-title' className='grid gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h2 className='text-lg font-semibold' id='site-performance-title'>
            {t('site.performance.title')}
          </h2>
          {performance?.sampled_at != null && (
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('site.performance.sampledAt', {
                time: fromUnixSeconds(performance.sampled_at).format(
                  'YYYY-MM-DD HH:mm:ss'
                ),
              })}
            </p>
          )}
        </div>
        <div
          aria-label={t('site.performance.timeRange')}
          className='flex items-center rounded-md border p-1'
          role='group'
        >
          {performanceRanges.map((hours) => (
            <Button
              aria-pressed={range === hours}
              className='min-h-8 px-2.5'
              key={hours}
              onClick={() => onRangeChange(hours)}
              size='sm'
              variant={range === hours ? 'secondary' : 'ghost'}
            >
              {t(dynamicI18nKey('site', performanceRangeLabels[hours]))}
            </Button>
          ))}
        </div>
      </div>

      {content}
    </section>
  )
}

function SiteMetadata({ site }: { site: SiteDetail }) {
  const { t } = useTranslation()
  const rows: Array<{ label: string; value: ReactNode }> = [
    {
      label: t('site.baseUrl'),
      value: (
        <a
          className='text-primary break-all hover:underline'
          href={site.base_url}
          rel='noreferrer'
          target='_blank'
        >
          {site.base_url}
        </a>
      ),
    },
    { label: t('site.remark'), value: formatDisplayValue(site.remark) },
    { label: t('site.version'), value: formatDisplayValue(site.version) },
    {
      label: t('site.systemName'),
      value: formatDisplayValue(site.system_name),
    },
    {
      label: t('site.exportEnabled'),
      value:
        site.data_export_enabled == null
          ? t('common.unknown')
          : t(
              dynamicI18nKey(
                'site',
                site.data_export_enabled ? 'common.yes' : 'common.no'
              )
            ),
    },
    {
      label: t('site.rootUserId'),
      value: formatDisplayValue(site.root_user_id),
    },
    {
      label: t('site.rootCreatedAt'),
      value: <TimestampValue timestamp={site.root_created_at} />,
    },
    {
      label: t('site.statisticsStartAt'),
      value: <TimestampValue timestamp={site.statistics_start_at} />,
    },
    {
      label: t('site.statisticsStartSource'),
      value:
        site.statistics_start_source == null
          ? formatDisplayValue(site.statistics_start_source)
          : t('site.statisticsStartSource.rootCreatedAt'),
    },
    {
      label: t('site.statisticsEndAt'),
      value: <TimestampValue timestamp={site.statistics_end_at} />,
    },
    {
      label: t('site.monitoringStartAt'),
      value: <TimestampValue timestamp={site.monitoring_start_at} />,
    },
    {
      label: t('site.lastProbeSuccessAt'),
      value: <TimestampValue timestamp={site.last_probe_success_at} />,
    },
  ]
  return (
    <section aria-labelledby='site-metadata-title' className='grid gap-3'>
      <div className='flex items-center gap-2'>
        <h2 className='text-lg font-semibold' id='site-metadata-title'>
          {t('site.detail.configuration')}
        </h2>
        <Badge variant='neutral'>{t('site.detail.immutableHistory')}</Badge>
      </div>
      <dl className='grid gap-x-6 gap-y-3 border-t pt-4 sm:grid-cols-2 lg:grid-cols-3'>
        {rows.map((row) => (
          <div className='min-w-0' key={row.label}>
            <dt className='text-muted-foreground text-xs'>{row.label}</dt>
            <dd className='mt-1 text-sm font-medium break-words'>
              {row.value}
            </dd>
          </div>
        ))}
      </dl>
      <div className='grid gap-3 border-t pt-4 sm:grid-cols-3'>
        <div>
          <p className='text-muted-foreground text-xs'>
            {t('site.rate.quotaPerUnit')}
          </p>
          <p className='mt-1 font-medium'>
            {formatNumericDisplayValue(site.rate.quota_per_unit)}
          </p>
        </div>
        <div>
          <p className='text-muted-foreground text-xs'>
            {t('site.rate.usdExchangeRate')}
          </p>
          <p className='mt-1 font-medium'>
            {formatNumericDisplayValue(site.rate.usd_exchange_rate)}
          </p>
        </div>
        <div>
          <p className='text-muted-foreground text-xs'>
            {t('site.rate.source')}
          </p>
          <p className='mt-1 font-medium'>
            {t(dynamicI18nKey('site', `site.rate.source.${site.rate.source}`))}
          </p>
        </div>
      </div>
    </section>
  )
}

function InstancePreview({
  error,
  instances,
  pending,
  siteId,
}: {
  error: boolean
  instances: SiteInstanceItem[]
  pending: boolean
  siteId: string
}) {
  const { t } = useTranslation()
  let content: ReactNode
  if (pending) {
    content = (
      <div
        aria-label={t('instance.loading')}
        className='grid gap-2 border-t pt-3 md:grid-cols-2 xl:grid-cols-3'
        role='status'
      >
        {Array.from({ length: 3 }, (_, index) => (
          <div className='bg-muted h-24 animate-pulse rounded-md' key={index} />
        ))}
      </div>
    )
  } else if (error) {
    content = (
      <p className='text-destructive text-sm'>{t('instance.loadError')}</p>
    )
  } else if (instances.length === 0) {
    content = (
      <p className='text-muted-foreground border-t py-4 text-sm'>
        {t('instance.empty')}
      </p>
    )
  } else {
    content = (
      <div className='grid gap-2 border-t pt-3 md:grid-cols-2 xl:grid-cols-3'>
        {instances.slice(0, 6).map((instance) => (
          <article
            className='border-border grid min-w-0 gap-2 border-b pb-3'
            key={instance.node_name}
          >
            <div className='flex items-center justify-between gap-2'>
              <h3 className='truncate font-medium' title={instance.node_name}>
                {instance.node_name}
              </h3>
              <InstanceStatusBadge status={instance.current_status} />
            </div>
            <p className='text-muted-foreground truncate text-xs'>
              {instance.hostname || '-'}
            </p>
            <div className='grid grid-cols-3 gap-2 text-xs'>
              <span>
                {t('metric.cpu')} <PercentValue value={instance.cpu_percent} />
              </span>
              <span>
                {t('metric.memory')}{' '}
                <PercentValue value={instance.memory_percent} />
              </span>
              <span>
                {t('metric.disk')}{' '}
                <PercentValue value={instance.disk_used_percent} />
              </span>
            </div>
          </article>
        ))}
      </div>
    )
  }
  return (
    <section aria-labelledby='site-instances-title' className='grid gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h2 className='text-lg font-semibold' id='site-instances-title'>
            {t('site.instances')}
          </h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('site.detail.instancesDescription')}
          </p>
        </div>
        <Link
          className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
          params={{ siteId }}
          to='/sites/$siteId/status'
        >
          <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
          {t('site.instanceStatus')}
        </Link>
      </div>
      {content}
    </section>
  )
}

function SiteDataDashboard({ siteId }: { siteId: string }) {
  const { t } = useTranslation()
  const [search, setSearch] = useState<StatisticsSearch>(() =>
    buildStatisticsSearch({})
  )
  const params = useMemo<EntityStatisticsParams>(
    () => ({
      end_timestamp: search.end,
      granularity: search.granularity,
      p: search.page,
      page_size: search.pageSize,
      sort_by: search.sort,
      sort_order: search.order,
      start_timestamp: search.start,
    }),
    [search]
  )
  const statisticsQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => getSiteStatistics(parseIdString(siteId), params),
    queryKey: siteKeys.statistics(siteId, params),
    staleTime: 5 * 60_000,
  })
  const response = statisticsQuery.data
  const contractValid =
    response == null ||
    (response.scope === 'site' &&
      response.granularity === search.granularity &&
      response.range.start_timestamp === search.start &&
      response.range.end_timestamp === search.end)
  const rangeTransition = Boolean(
    response &&
    !contractValid &&
    statisticsQuery.isPlaceholderData &&
    statisticsQuery.isFetching
  )
  const data = contractValid || rangeTransition ? response : undefined

  return (
    <section aria-labelledby='site-data-dashboard-title' className='grid gap-4'>
      <div>
        <h2 className='text-lg font-semibold' id='site-data-dashboard-title'>
          {t('site.actions.stats')}
        </h2>
      </div>
      <EntityStatistics
        data={data}
        entityId={parseIdString(siteId)}
        error={
          statisticsQuery.isError ||
          (!contractValid && !rangeTransition && !statisticsQuery.isFetching)
        }
        fetching={statisticsQuery.isFetching}
        loading={statisticsQuery.isPending}
        onRetry={() => void statisticsQuery.refetch()}
        onSearchChange={(changes) =>
          setSearch((current) =>
            buildStatisticsSearch({ ...current, ...changes })
          )
        }
        rangeTransition={rangeTransition}
        scope='site'
        search={search}
      />
    </section>
  )
}

export function SiteDetailPage({
  onDeleted,
  onSearchChange,
  search,
  siteId,
}: SiteDetailPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.user)
  const isAdmin = currentUser?.role === 'admin'
  const [dialogState, setDialogState] = useState<SiteDialogState | null>(null)
  const [performanceRange, setPerformanceRange] =
    useState<(typeof performanceRanges)[number]>(24)
  const validSiteId = isIdString(siteId)
  const detailQuery = useQuery({
    enabled: validSiteId,
    queryFn: () => getSite(parseIdString(siteId)),
    queryKey: siteKeys.detail(siteId),
    refetchInterval: (query) =>
      query.state.data?.statistics_status === 'backfilling' ? 5_000 : 60_000,
    staleTime: 30_000,
  })
  const instancesQuery = useQuery({
    enabled: validSiteId,
    queryFn: () => listSiteInstances(parseIdString(siteId)),
    queryKey: siteKeys.instances(siteId),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })
  const performanceQuery = useQuery({
    enabled: validSiteId,
    queryFn: () => getSitePerformance(parseIdString(siteId), performanceRange),
    queryKey: siteKeys.performance(siteId, performanceRange),
    staleTime: 60_000,
  })
  const site = detailQuery.data

  const retry = () => {
    void detailQuery.refetch()
    void instancesQuery.refetch()
    void performanceQuery.refetch()
  }
  const invalidate = (action: SiteAction) => {
    void queryClient.invalidateQueries({ queryKey: siteKeys.all })
    if (action === 'delete') onDeleted()
  }

  const actions = site ? (
    <>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildFinancialOperationsSearch({})}
        to='/sites/$siteId/financial-operations'
      >
        <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
        {t('site.actions.financialOperations')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildPerformanceHistorySearch({})}
        to='/sites/$siteId/performance-history'
      >
        <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
        {t('site.actions.performanceHistory')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildChannelInventorySearch({})}
        to='/sites/$siteId/channel-inventory'
      >
        <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
        {t('site.actions.channelInventory')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildUserInventorySearch({})}
        to='/sites/$siteId/user-inventory'
      >
        <HugeiconsIcon icon={UserGroupIcon} strokeWidth={2} />
        {t('site.actions.userInventory')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildUpstreamTaskSearch({})}
        to='/sites/$siteId/upstream-tasks'
      >
        <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
        {t('site.actions.upstreamTasks')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildModelCatalogSearch({})}
        to='/sites/$siteId/model-catalog'
      >
        <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
        {t('site.actions.modelCatalog')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildRankingSearch({})}
        to='/sites/$siteId/rankings'
      >
        <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
        {t('site.actions.rankings')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildPricingGroupSearch({})}
        to='/sites/$siteId/pricing-groups'
      >
        <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
        {t('site.actions.pricingGroups')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildSubscriptionPlanSearch({})}
        to='/sites/$siteId/subscription-plans'
      >
        <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
        {t('site.actions.subscriptionPlans')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildSystemTaskSearch({})}
        to='/sites/$siteId/system-tasks'
      >
        <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
        {t('site.actions.systemTasks')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildLogSearch({})}
        to='/sites/$siteId/logs'
      >
        <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
        {t('site.actions.logs')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        search={buildStatisticsSearch({})}
        to='/sites/$siteId/stats'
      >
        <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
        {t('site.actions.stats')}
      </Link>
      <Link
        className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
        params={{ siteId }}
        to='/sites/$siteId/status'
      >
        <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
        {t('site.instanceStatus')}
      </Link>
      {isAdmin && (
        <SiteActions
          onAction={(action, selectedSite) =>
            setDialogState({ action, site: selectedSite })
          }
          site={site}
        />
      )}
    </>
  ) : undefined

  let detailContent: ReactNode
  if (!validSiteId || (detailQuery.isError && !site)) {
    detailContent = (
      <ErrorState
        description={t(
          dynamicI18nKey(
            'site',
            validSiteId
              ? 'site.detail.loadErrorDescription'
              : 'site.detail.invalidId'
          )
        )}
        onRetry={validSiteId ? retry : undefined}
        title={t('site.detail.loadError')}
      />
    )
  } else if (detailQuery.isPending || !site) {
    detailContent = <LoadingState message={t('site.detail.loading')} />
  } else {
    detailContent = (
      <>
        {detailQuery.isRefetchError && (
          <section
            className='border-warning/40 bg-warning/10 flex flex-wrap items-center justify-between gap-3 rounded-md border p-3'
            role='status'
          >
            <div>
              <h2 className='text-sm font-medium'>
                {t('site.detail.refreshError')}
              </h2>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('site.detail.refreshErrorDescription')}
              </p>
            </div>
            <Button
              onClick={() => void detailQuery.refetch()}
              variant='outline'
            >
              <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
              {t('common.retry')}
            </Button>
          </section>
        )}
        <section className='grid gap-3'>
          <h2 className='sr-only'>{t('site.statuses')}</h2>
          <SiteStatusBadges site={site} />
        </section>
        <DetailSummary site={site} />
        <PerformanceHealth
          error={performanceQuery.isError}
          onRangeChange={setPerformanceRange}
          pending={performanceQuery.isPending}
          performance={performanceQuery.data}
          range={performanceRange}
        />
        <div className='grid gap-4 lg:grid-cols-2'>
          <CompletenessAlert completeness={site.completeness} />
          <BackfillProgress backfill={site.backfill} />
        </div>
        <SiteDataDashboard siteId={siteId} />
        <SiteMetadata site={site} />
        <InstancePreview
          error={instancesQuery.isError && instancesQuery.data == null}
          instances={instancesQuery.data ?? []}
          pending={instancesQuery.isPending}
          siteId={siteId}
        />
      </>
    )
  }

  return (
    <SectionPageLayout
      actions={actions}
      description={site?.base_url ?? t('site.detail.description')}
      title={site?.name ?? t('site.detail.title')}
    >
      <div className='grid min-w-0 gap-8'>
        <DetailBackLink
          render={
            <Link
              search={{
                auth: [],
                health: [],
                management: [],
                online: [],
                statistics: [],
              }}
              to='/sites'
            />
          }
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          {t('site.backToList')}
        </DetailBackLink>

        {detailContent}

        <CollectionRunsPanel
          isAdmin={Boolean(isAdmin)}
          onSearchChange={onSearchChange}
          search={search}
          siteId={siteId}
        />
      </div>

      <SiteDialogs
        onClose={() => setDialogState(null)}
        onSaved={invalidate}
        state={dialogState}
      />
    </SectionPageLayout>
  )
}
