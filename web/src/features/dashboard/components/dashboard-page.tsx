import {
  Alert02Icon,
  Analytics01Icon,
  Chart01Icon,
  Pulse01Icon,
  RankingIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { CompletenessAlert } from '@/components/data/completeness-alert'
import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { translateMessageRef } from '@/lib/message-ref'

import {
  AmountValue,
  MetricTrendChart,
} from '../../statistics/components/entity-statistics'
import { buildStatisticsSearch } from '../../statistics/search'
import type { StatisticsMetric, StatisticsSearch } from '../../statistics/types'
import {
  getDashboardHealth,
  getDashboardSummary,
  getDashboardTop,
  getDashboardTrend,
} from '../api'
import { dashboardKeys } from '../query-keys'
import type {
  DashboardHealth,
  DashboardRankingItem,
  DashboardSummary,
  DashboardTopMetric,
  DashboardTopType,
  DashboardTrend,
} from '../types'

type DashboardQueryState<T> = {
  data?: T
  error: boolean
  fetching: boolean
  loading: boolean
  retry: () => void
}

function DashboardPanel({
  children,
  empty,
  icon,
  id,
  state,
  title,
}: {
  children: ReactNode
  empty?: boolean
  icon: typeof Analytics01Icon
  id: string
  state: DashboardQueryState<unknown>
  title: string
}) {
  const { t } = useTranslation()
  let content: ReactNode
  if (state.loading && !state.data) {
    content = (
      <div
        aria-hidden='true'
        className='bg-muted h-40 animate-pulse rounded-md'
      />
    )
  } else if (state.error && !state.data) {
    content = (
      <ErrorState
        className='min-h-40'
        description={t('dashboard.block.loadErrorDescription')}
        onRetry={state.retry}
        title={t('dashboard.block.loadError')}
      />
    )
  } else if (empty) {
    content = (
      <EmptyState
        className='min-h-40'
        description={t('dashboard.block.emptyDescription')}
        title={t('dashboard.block.empty')}
      />
    )
  } else {
    content = (
      <>
        {state.error && (
          <p className='border-warning/40 bg-warning/10 rounded-md border p-3 text-sm'>
            {t('dashboard.block.stale')}
          </p>
        )}
        {children}
      </>
    )
  }
  return (
    <section
      aria-labelledby={`dashboard-${id}`}
      className='border-border grid min-w-0 content-start gap-4 border-b pb-7'
    >
      <header className='flex min-w-0 flex-wrap items-center justify-between gap-3'>
        <h2
          className='flex min-w-0 items-center gap-2 text-base font-semibold break-words'
          id={`dashboard-${id}`}
        >
          <HugeiconsIcon icon={icon} size={20} strokeWidth={2} />
          {title}
        </h2>
        {state.fetching && !state.loading && (
          <span className='text-muted-foreground text-xs' role='status'>
            {t('dashboard.refreshing')}
          </span>
        )}
      </header>
      {content}
    </section>
  )
}

function MetricCell({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className='min-w-0 p-4'>
      <dt className='text-muted-foreground text-xs break-words'>{label}</dt>
      <dd className='mt-1 text-xl font-semibold break-words'>{value}</dd>
    </div>
  )
}

function TodayOperations({ data }: { data: DashboardSummary }) {
  const { t } = useTranslation()
  return (
    <div className='grid min-w-0 gap-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <DataStatusBadge status={data.today.data_status} />
          <span className='text-muted-foreground text-xs'>
            {data.today.is_final
              ? t('statistics.final.final')
              : t('statistics.final.provisional')}
          </span>
        </div>
        <DataFreshness
          labelKey='dashboard.businessAsOf'
          timestamp={data.today.as_of}
        />
      </div>
      {data.today.reason && data.today.data_status !== 'complete' && (
        <p className='border-warning/40 bg-warning/10 rounded-md border p-3 text-sm'>
          {translateMessageRef(data.today.reason)}
        </p>
      )}
      <dl className='border-border grid overflow-hidden rounded-md border sm:grid-cols-2 xl:grid-cols-5 [&>div]:border-r [&>div]:border-b'>
        <MetricCell
          label={t('dashboard.today.requests')}
          value={<MetricValue value={data.today.request_count} />}
        />
        <MetricCell
          label={t('dashboard.today.quota')}
          value={<MetricValue value={data.today.quota} />}
        />
        <MetricCell
          label={t('dashboard.today.amount')}
          value={
            <AmountValue
              display='cny'
              siteBreakdown={data.today.site_breakdown}
            />
          }
        />
        <MetricCell
          label={t('dashboard.today.tokens')}
          value={<MetricValue value={data.today.token_used} />}
        />
        <MetricCell
          label={t('dashboard.today.activeAccounts')}
          value={<MetricValue value={data.active_accounts_today} />}
        />
      </dl>
      <dl className='grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2 xl:grid-cols-4'>
        <div>
          <dt className='text-muted-foreground'>
            {t('dashboard.entities.sites')}
          </dt>
          <dd className='mt-1 font-medium'>
            {t('dashboard.entities.siteValue', {
              offline: data.offline_site_count,
              online: data.online_site_count,
              total: data.site_count,
            })}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {t('dashboard.entities.customers')}
          </dt>
          <dd className='mt-1 font-medium'>{data.customer_count}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {t('dashboard.entities.accounts')}
          </dt>
          <dd className='mt-1 font-medium'>{data.managed_account_count}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {t('dashboard.entities.instances')}
          </dt>
          <dd className='mt-1 font-medium'>
            {data.instance_count == null || data.online_instance_count == null
              ? t('data.unavailableValue')
              : t('dashboard.entities.instanceValue', {
                  online: data.online_instance_count,
                  total: data.instance_count,
                })}
          </dd>
        </div>
      </dl>
      <section
        aria-labelledby='resource-completeness-title'
        className='border-border grid gap-3 border-t pt-4'
      >
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='flex flex-wrap items-center gap-2'>
            <h3
              className='text-sm font-medium'
              id='resource-completeness-title'
            >
              {t('dashboard.entities.resourceCompleteness')}
            </h3>
            <DataStatusBadge status={data.resource_data_status} />
            <span className='text-muted-foreground text-xs'>
              {t('dashboard.realtime.coverage', {
                complete: data.resource_complete_site_count,
                expected: data.resource_expected_site_count,
              })}
            </span>
          </div>
          <DataFreshness
            labelKey='dashboard.currentAsOf'
            timestamp={data.resource_as_of}
          />
        </div>
        {data.resource_reason && (
          <p className='border-warning/40 bg-warning/10 rounded-md border p-3 text-sm'>
            {translateMessageRef(data.resource_reason)}
          </p>
        )}
        {data.resource_stale_site_ids.length > 0 && (
          <p className='text-muted-foreground text-xs break-all'>
            {t('dashboard.entities.resourceStaleSites', {
              ids: data.resource_stale_site_ids.join(', '),
            })}
          </p>
        )}
      </section>
    </div>
  )
}

function RealtimeThroughput({ data }: { data: DashboardSummary }) {
  const { t } = useTranslation()
  return (
    <div className='grid gap-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <DataStatusBadge status={data.realtime_data_status} />
          <span className='text-muted-foreground text-xs'>
            {t('dashboard.realtime.coverage', {
              complete: data.realtime_complete_site_count,
              expected: data.realtime_expected_site_count,
            })}
          </span>
        </div>
        <DataFreshness
          labelKey='dashboard.currentAsOf'
          timestamp={data.realtime_as_of}
        />
      </div>
      <dl className='border-border grid overflow-hidden rounded-md border sm:grid-cols-2 [&>div]:border-r'>
        <MetricCell
          label={t('dashboard.realtime.rpm')}
          value={<MetricValue value={data.rpm} />}
        />
        <MetricCell
          label={t('dashboard.realtime.tpm')}
          value={<MetricValue value={data.tpm} />}
        />
      </dl>
      {data.realtime_reason && (
        <p className='text-muted-foreground text-sm'>
          {translateMessageRef(data.realtime_reason)}
        </p>
      )}
      {data.stale_site_ids.length > 0 && (
        <p className='text-muted-foreground text-xs break-all'>
          {t('dashboard.realtime.staleSites', {
            ids: data.stale_site_ids.join(', '),
          })}
        </p>
      )}
    </div>
  )
}

function trendSearch(
  data: DashboardTrend,
  metric: StatisticsMetric
): StatisticsSearch {
  return {
    accountIds: [],
    channelKeys: [],
    customerIds: [],
    display: 'quota',
    end: data.at(-1)?.bucket_end ?? 0,
    granularity: 'day',
    metric,
    models: [],
    nodeNames: [],
    order: 'asc',
    page: 1,
    pageSize: 30,
    siteIds: [],
    sort: 'bucket_start',
    start: data[0]?.bucket_start ?? 0,
    tokenKeys: [],
    useGroups: [],
    view: 'chart',
  }
}

function ThirtyDayTrend({ data }: { data: DashboardTrend }) {
  const { t } = useTranslation()
  const [metric, setMetric] = useState<StatisticsMetric>('request_count')
  const search = useMemo(() => trendSearch(data, metric), [data, metric])
  return (
    <div className='grid min-w-0 gap-4'>
      <div className='flex flex-wrap gap-2' role='group'>
        {(['request_count', 'quota', 'token_used'] as const).map((value) => (
          <Button
            aria-pressed={metric === value}
            key={value}
            onClick={() => setMetric(value)}
            size='sm'
            variant={metric === value ? 'secondary' : 'outline'}
          >
            {t(dynamicI18nKey('statistics', `statistics.metric.${value}`))}
          </Button>
        ))}
      </div>
      <MetricTrendChart data={data} search={search} />
      <Button
        className='w-fit'
        render={
          <Link search={buildStatisticsSearch({})} to='/statistics/global' />
        }
        variant='ghost'
      >
        {t('dashboard.openStatistics')}
      </Button>
    </div>
  )
}

function RankingValue({
  item,
  metric,
}: {
  item: DashboardRankingItem
  metric: DashboardTopMetric
}) {
  if (metric === 'quota') {
    return (
      <div className='text-right'>
        <MetricValue compact value={item.value} />
        <div className='text-muted-foreground mt-1 text-xs'>
          <AmountValue display='cny' siteBreakdown={item.site_breakdown} />
        </div>
      </div>
    )
  }
  return <MetricValue compact value={item.value} />
}

function Ranking({
  data,
  limit,
  metric,
  onLimitChange,
  onMetricChange,
  onTypeChange,
  type,
}: {
  data: DashboardRankingItem[]
  limit: number
  metric: DashboardTopMetric
  onLimitChange: (value: number) => void
  onMetricChange: (value: DashboardTopMetric) => void
  onTypeChange: (value: DashboardTopType) => void
  type: DashboardTopType
}) {
  const { t } = useTranslation()
  const types: DashboardTopType[] = ['site', 'customer', 'model', 'channel']
  const typeLabels: Record<DashboardTopType, string> = {
    channel: t('dashboard.ranking.type.channel'),
    customer: t('dashboard.ranking.type.customer'),
    model: t('dashboard.ranking.type.model'),
    site: t('dashboard.ranking.type.site'),
  }
  return (
    <div className='grid gap-4'>
      <div className='flex flex-wrap items-end justify-between gap-3'>
        <Tabs
          onValueChange={(value) => onTypeChange(value as DashboardTopType)}
          value={type}
        >
          <TabsList aria-label={t('dashboard.ranking.dimension')}>
            {types.map((value) => (
              <TabsTrigger key={value} value={value}>
                {typeLabels[value]}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <div className='flex flex-wrap items-end gap-3'>
          <fieldset className='grid gap-1'>
            <legend className='text-muted-foreground text-xs'>
              {t('dashboard.ranking.metric')}
            </legend>
            <div className='border-border flex rounded-md border p-0.5'>
              {(['request_count', 'quota'] as const).map((value) => (
                <Button
                  aria-pressed={metric === value}
                  key={value}
                  onClick={() => onMetricChange(value)}
                  size='sm'
                  variant={metric === value ? 'secondary' : 'ghost'}
                >
                  {t(
                    dynamicI18nKey('statistics', `statistics.metric.${value}`)
                  )}
                </Button>
              ))}
            </div>
          </fieldset>
          <label className='grid gap-1 text-sm'>
            <span className='text-muted-foreground text-xs'>
              {t('dashboard.ranking.limit')}
            </span>
            <Input
              className='w-20'
              max={20}
              min={1}
              onChange={(event) => {
                const value = Number(event.target.value)
                if (Number.isInteger(value) && value >= 1 && value <= 20) {
                  onLimitChange(value)
                }
              }}
              type='number'
              value={limit}
            />
          </label>
        </div>
      </div>
      <div
        aria-labelledby={`dashboard-ranking-tab-${type}`}
        id='dashboard-ranking-panel'
        role='tabpanel'
      >
        <ol className='divide-border divide-y'>
          {data.map((item, index) => (
            <li
              className='grid min-w-0 grid-cols-[2rem_minmax(0,1fr)_auto] items-center gap-3 py-3'
              key={`${item.dimension_type}-${item.dimension_id}`}
            >
              <span className='text-muted-foreground text-center text-sm'>
                {index + 1}
              </span>
              <div className='min-w-0'>
                <p className='font-medium break-words'>{item.dimension_name}</p>
                <div className='mt-1 flex flex-wrap items-center gap-2'>
                  <DataStatusBadge status={item.data_status} />
                  {!item.is_final && (
                    <span className='text-muted-foreground text-xs'>
                      {t('statistics.final.provisional')}
                    </span>
                  )}
                </div>
              </div>
              <RankingValue item={item} metric={metric} />
            </li>
          ))}
        </ol>
      </div>
    </div>
  )
}

function HealthAndCompleteness({ data }: { data: DashboardHealth }) {
  const { t } = useTranslation()
  return (
    <div className='grid min-w-0 gap-6'>
      <div className='grid gap-3 sm:grid-cols-3'>
        <div className='border-border rounded-md border p-4'>
          <p className='text-muted-foreground text-xs'>
            {t('dashboard.health.firing')}
          </p>
          <p className='mt-1 text-xl font-semibold'>
            {data.firing_alert_count}
          </p>
        </div>
        <div className='border-destructive/30 rounded-md border p-4'>
          <p className='text-muted-foreground text-xs'>
            {t('dashboard.health.critical')}
          </p>
          <p className='mt-1 text-xl font-semibold'>
            {data.critical_alert_count}
          </p>
        </div>
        <div className='border-warning/40 rounded-md border p-4'>
          <p className='text-muted-foreground text-xs'>
            {t('dashboard.health.warning')}
          </p>
          <p className='mt-1 text-xl font-semibold'>
            {data.warning_alert_count}
          </p>
        </div>
      </div>
      <dl className='border-border grid gap-4 border-y py-4 text-sm sm:grid-cols-3'>
        <div className='grid content-start gap-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('dashboard.health.yesterdayValidation')}
          </dt>
          <dd className='flex flex-wrap items-center gap-2'>
            <DataStatusBadge status={data.yesterday_validation_status} />
            <span className='text-muted-foreground text-xs'>
              {data.is_final
                ? t('statistics.final.final')
                : t('statistics.final.provisional')}
            </span>
          </dd>
        </div>
        <div className='grid content-start gap-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('dashboard.health.authExpiredSites')}
          </dt>
          <dd className='font-medium break-all'>
            {data.auth_expired_site_ids.length > 0
              ? data.auth_expired_site_ids.join(', ')
              : t('common.none')}
          </dd>
        </div>
        <div className='grid content-start gap-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('dashboard.health.statisticsNotReadySites')}
          </dt>
          <dd className='font-medium break-all'>
            {data.statistics_not_ready_site_ids.length > 0
              ? data.statistics_not_ready_site_ids.join(', ')
              : t('common.none')}
          </dd>
        </div>
      </dl>
      {data.reason && (
        <p
          className='border-warning/40 bg-warning/10 rounded-md border p-3 text-sm'
          data-testid='dashboard-health-reason'
        >
          {translateMessageRef(data.reason)}
        </p>
      )}
      <div>
        <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
          <h3 className='font-medium'>{t('dashboard.health.sites')}</h3>
          <DataFreshness
            labelKey='dashboard.currentAsOf'
            timestamp={data.as_of}
          />
        </div>
        {data.sites.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t('dashboard.health.noSites')}
          </p>
        ) : (
          <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-3'>
            {data.sites.map((site) => (
              <Link
                className='border-border hover:bg-muted/50 grid min-w-0 gap-2 rounded-md border p-3 transition-colors'
                key={site.site_id}
                params={{ siteId: site.site_id }}
                to='/sites/$siteId'
              >
                <span className='font-medium break-words'>
                  {site.site_name}
                </span>
                <div className='flex flex-wrap gap-1'>
                  <Badge variant='neutral'>
                    {t(
                      dynamicI18nKey(
                        'site',
                        `site.online.${site.online_status}`
                      )
                    )}
                  </Badge>
                  <Badge variant='neutral'>
                    {t(dynamicI18nKey('site', `site.auth.${site.auth_status}`))}
                  </Badge>
                  <Badge variant='neutral'>
                    {t(
                      dynamicI18nKey(
                        'site',
                        `site.health.${site.health_status}`
                      )
                    )}
                  </Badge>
                  <Badge variant='neutral'>
                    {t(
                      dynamicI18nKey(
                        'site',
                        `site.statistics.${site.statistics_status}`
                      )
                    )}
                  </Badge>
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>
      <CompletenessAlert completeness={data.completeness} />
      <div className='grid gap-3'>
        <h3 className='font-medium'>{t('dashboard.health.latestAlerts')}</h3>
        {data.latest_alerts.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t('dashboard.health.noAlerts')}
          </p>
        ) : (
          <ul className='divide-border divide-y'>
            {data.latest_alerts.slice(0, 5).map((alert) => (
              <li className='grid min-w-0 gap-1 py-3' key={alert.id}>
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge
                    variant={
                      alert.level === 'critical' ? 'destructive' : 'warning'
                    }
                  >
                    {alert.level === 'critical'
                      ? t('dashboard.health.critical')
                      : t('dashboard.health.warning')}
                  </Badge>
                  <span className='font-medium break-words'>
                    {alert.target_name}
                  </span>
                </div>
                <p className='text-muted-foreground text-sm break-words'>
                  {translateMessageRef(alert.message)}
                </p>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}

export function DashboardPage() {
  const { t } = useTranslation()
  const [topType, setTopType] = useState<DashboardTopType>('customer')
  const [topMetric, setTopMetric] =
    useState<DashboardTopMetric>('request_count')
  const [topLimit, setTopLimit] = useState(5)
  const summaryQuery = useQuery({
    queryFn: getDashboardSummary,
    queryKey: dashboardKeys.summary(),
    staleTime: 5 * 60_000,
  })
  const trendQuery = useQuery({
    queryFn: () => getDashboardTrend(30),
    queryKey: dashboardKeys.trend(30),
    staleTime: 5 * 60_000,
  })
  const siteTopQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => getDashboardTop('site', topMetric, topLimit),
    queryKey: dashboardKeys.top('site', topMetric, topLimit),
    staleTime: 5 * 60_000,
  })
  const customerTopQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => getDashboardTop('customer', topMetric, topLimit),
    queryKey: dashboardKeys.top('customer', topMetric, topLimit),
    staleTime: 5 * 60_000,
  })
  const modelTopQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => getDashboardTop('model', topMetric, topLimit),
    queryKey: dashboardKeys.top('model', topMetric, topLimit),
    staleTime: 5 * 60_000,
  })
  const channelTopQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => getDashboardTop('channel', topMetric, topLimit),
    queryKey: dashboardKeys.top('channel', topMetric, topLimit),
    staleTime: 5 * 60_000,
  })
  const topQuery = {
    channel: channelTopQuery,
    customer: customerTopQuery,
    model: modelTopQuery,
    site: siteTopQuery,
  }[topType]
  const healthQuery = useQuery({
    queryFn: getDashboardHealth,
    queryKey: dashboardKeys.health(),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })
  const summaryState: DashboardQueryState<DashboardSummary> = {
    data: summaryQuery.data,
    error: summaryQuery.isError,
    fetching: summaryQuery.isFetching,
    loading: summaryQuery.isPending,
    retry: () => void summaryQuery.refetch(),
  }
  const trendState: DashboardQueryState<DashboardTrend> = {
    data: trendQuery.data,
    error: trendQuery.isError,
    fetching: trendQuery.isFetching,
    loading: trendQuery.isPending,
    retry: () => void trendQuery.refetch(),
  }
  const topState: DashboardQueryState<DashboardRankingItem[]> = {
    data: topQuery.data,
    error: topQuery.isError,
    fetching: topQuery.isFetching,
    loading: topQuery.isPending,
    retry: () => void topQuery.refetch(),
  }
  const healthState: DashboardQueryState<DashboardHealth> = {
    data: healthQuery.data,
    error: healthQuery.isError,
    fetching: healthQuery.isFetching,
    loading: healthQuery.isPending,
    retry: () => void healthQuery.refetch(),
  }
  return (
    <SectionPageLayout
      description={t('dashboard.description')}
      title={t('dashboard.title')}
    >
      <div className='grid min-w-0 gap-7'>
        <DashboardPanel
          icon={Analytics01Icon}
          id='today'
          state={summaryState}
          title={t('dashboard.section.today')}
        >
          {summaryQuery.data && <TodayOperations data={summaryQuery.data} />}
        </DashboardPanel>
        <DashboardPanel
          icon={Pulse01Icon}
          id='realtime'
          state={summaryState}
          title={t('dashboard.section.realtime')}
        >
          {summaryQuery.data && <RealtimeThroughput data={summaryQuery.data} />}
        </DashboardPanel>
        <DashboardPanel
          empty={trendQuery.data?.length === 0}
          icon={Chart01Icon}
          id='trend'
          state={trendState}
          title={t('dashboard.section.trend')}
        >
          {trendQuery.data && <ThirtyDayTrend data={trendQuery.data} />}
        </DashboardPanel>
        <DashboardPanel
          empty={topQuery.data?.length === 0}
          icon={RankingIcon}
          id='ranking'
          state={topState}
          title={t('dashboard.section.ranking')}
        >
          {topQuery.data && (
            <Ranking
              data={topQuery.data}
              limit={topLimit}
              metric={topMetric}
              onLimitChange={setTopLimit}
              onMetricChange={setTopMetric}
              onTypeChange={setTopType}
              type={topType}
            />
          )}
        </DashboardPanel>
        <DashboardPanel
          icon={Alert02Icon}
          id='health'
          state={healthState}
          title={t('dashboard.section.health')}
        >
          {healthQuery.data && (
            <HealthAndCompleteness data={healthQuery.data} />
          )}
        </DashboardPanel>
      </div>
    </SectionPageLayout>
  )
}
