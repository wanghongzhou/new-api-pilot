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
import { SelectControl } from '@/components/ui/select-control'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { BEIJING_TIMEZONE, dayjs } from '@/lib/dayjs'
import { translateMessageRef } from '@/lib/message-ref'
import { cn } from '@/lib/utils'

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
import { isDashboardProblemSite } from '../health'
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
  className,
  empty,
  icon,
  id,
  state,
  title,
}: {
  children: ReactNode
  className?: string
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
      <>
        <span className='sr-only' role='status'>
          {t('common.loading')}
        </span>
        <div
          aria-hidden='true'
          className='bg-muted h-40 animate-pulse rounded-md'
        />
      </>
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
      <>
        {state.error && (
          <p className='border-warning/40 bg-warning/10 rounded-md border p-3 text-sm'>
            {t('dashboard.block.stale')}
          </p>
        )}
        <EmptyState
          className='min-h-40'
          description={t('dashboard.block.emptyDescription')}
          title={t('dashboard.block.empty')}
        />
      </>
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
      className={cn(
        'border-border bg-(--data-table-card-bg,var(--table-row)) grid min-w-0 content-start gap-4 rounded-lg border p-4',
        className
      )}
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
    <div className='border-border bg-background/60 min-w-0 rounded-lg border px-3 py-3.5'>
      <dt className='text-muted-foreground text-xs break-words'>{label}</dt>
      <dd className='mt-1.5 text-xl font-semibold break-words'>{value}</dd>
    </div>
  )
}

function StaleSiteLinks({ ids, label }: { ids: string[]; label: string }) {
  return (
    <div className='text-muted-foreground flex flex-wrap items-center gap-1.5 text-xs'>
      <span>{label}</span>
      {ids.map((siteId) => (
        <Link
          className='border-border bg-background hover:bg-muted rounded border px-1.5 py-0.5 font-mono transition-colors'
          key={siteId}
          params={{ siteId }}
          to='/sites/$siteId'
        >
          {siteId}
        </Link>
      ))}
    </div>
  )
}

function OperationalAttention({ data }: { data: DashboardHealth }) {
  const { t } = useTranslation()
  const offlineSites = data.sites.filter(
    (site) =>
      site.management_status === 'active' && site.online_status !== 'online'
  ).length
  const alertSearch = {
    level: [],
    ruleCategory: [],
    ruleLevel: [],
    status: [],
    targetType: [],
  }
  const siteSearch = {
    auth: [],
    health: [],
    management: [],
    online: [],
    statistics: [],
  }
  const items = [
    {
      destination: 'alerts',
      label: t('dashboard.health.firing'),
      search: { ...alertSearch, status: ['firing'] },
      tone: data.firing_alert_count > 0 ? 'warning' : 'neutral',
      value: data.firing_alert_count,
    },
    {
      destination: 'alerts',
      label: t('dashboard.health.critical'),
      search: { ...alertSearch, level: ['critical'], status: ['firing'] },
      tone: data.critical_alert_count > 0 ? 'critical' : 'neutral',
      value: data.critical_alert_count,
    },
    {
      destination: 'alerts',
      label: t('dashboard.health.warning'),
      search: { ...alertSearch, level: ['warning'], status: ['firing'] },
      tone: data.warning_alert_count > 0 ? 'warning' : 'neutral',
      value: data.warning_alert_count,
    },
    {
      destination: 'sites',
      label: t('dashboard.attention.offlineSites'),
      search: { ...siteSearch, management: ['active'], online: ['offline'] },
      tone: offlineSites > 0 ? 'critical' : 'neutral',
      value: offlineSites,
    },
    {
      destination: 'sites',
      label: t('dashboard.health.authExpiredSites'),
      search: { ...siteSearch, auth: ['expired'], management: ['active'] },
      tone: data.auth_expired_site_ids.length > 0 ? 'warning' : 'neutral',
      value: data.auth_expired_site_ids.length,
    },
    {
      destination: 'sites',
      label: t('dashboard.health.statisticsNotReadySites'),
      search: {
        ...siteSearch,
        management: ['active'],
        statistics: [
          'pending_config',
          'backfilling',
          'partial',
          'error',
          'paused',
        ],
      },
      tone:
        data.statistics_not_ready_site_ids.length > 0 ? 'warning' : 'neutral',
      value: data.statistics_not_ready_site_ids.length,
    },
  ] as const

  return (
    <div className='grid gap-4'>
      <div className='grid gap-2 sm:grid-cols-2 xl:grid-cols-6'>
        {items.map((item) => {
          const content = (
            <>
              <span className='text-muted-foreground text-xs'>
                {item.label}
              </span>
              <span
                className={cn(
                  'text-xl font-semibold',
                  item.tone === 'critical' && 'text-destructive',
                  item.tone === 'warning' && 'text-warning-foreground'
                )}
              >
                {item.value}
              </span>
            </>
          )
          const className = cn(
            'border-border bg-background/60 grid min-h-20 content-center gap-1 rounded-lg border px-3 py-2.5 transition-colors',
            item.tone === 'critical' &&
              'border-destructive/35 bg-destructive/5',
            item.tone === 'warning' && 'border-warning/40 bg-warning/10'
          )
          if (item.destination === 'alerts') {
            return (
              <Link
                className={cn(className, 'hover:bg-muted/70')}
                key={item.label}
                search={item.search}
                to='/alerts'
              >
                {content}
              </Link>
            )
          }
          return (
            <Link
              className={cn(className, 'hover:bg-muted/70')}
              key={item.label}
              search={item.search}
              to='/sites'
            >
              {content}
            </Link>
          )
        })}
      </div>
      <div className='flex flex-wrap items-center justify-between gap-3 border-t pt-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <span className='text-muted-foreground text-xs'>
            {t('dashboard.health.yesterdayValidation')}
          </span>
          <DataStatusBadge status={data.yesterday_validation_status} />
          <span className='text-muted-foreground text-xs'>
            {data.is_final
              ? t('statistics.final.final')
              : t('statistics.final.provisional')}
          </span>
        </div>
        <DataFreshness
          labelKey='dashboard.currentAsOf'
          timestamp={data.as_of}
        />
      </div>
      {data.reason && (
        <p className='border-warning/40 bg-warning/10 rounded-md border p-3 text-sm'>
          {translateMessageRef(data.reason)}
        </p>
      )}
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
      <dl className='grid gap-2 sm:grid-cols-2 xl:grid-cols-5'>
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
      <dl className='grid gap-2 sm:grid-cols-2'>
        <MetricCell
          label={t('dashboard.realtime.rpm')}
          value={<MetricValue value={data.rpm} />}
        />
        <MetricCell
          label={t('dashboard.realtime.tpm')}
          value={<MetricValue value={data.tpm} />}
        />
      </dl>
      <div className='grid gap-2 sm:grid-cols-2'>
        <Link
          className='border-border hover:bg-muted/70 grid rounded-lg border px-3 py-2.5 transition-colors'
          search={{
            auth: [],
            health: [],
            management: [],
            online: [],
            statistics: [],
          }}
          to='/sites'
        >
          <span className='text-muted-foreground text-xs'>
            {t('dashboard.entities.sites')}
          </span>
          <span className='mt-1 font-medium'>
            {t('dashboard.entities.siteValue', {
              offline: data.offline_site_count,
              online: data.online_site_count,
              total: data.site_count,
            })}
          </span>
        </Link>
        <Link
          className='border-border hover:bg-muted/70 grid rounded-lg border px-3 py-2.5 transition-colors'
          search={{ status: [] }}
          to='/customers'
        >
          <span className='text-muted-foreground text-xs'>
            {t('dashboard.entities.customers')}
          </span>
          <span className='mt-1 font-medium'>{data.customer_count}</span>
        </Link>
        <Link
          className='border-border hover:bg-muted/70 grid rounded-lg border px-3 py-2.5 transition-colors'
          search={{ managedStatus: [], remoteState: [], remoteStatus: [] }}
          to='/accounts'
        >
          <span className='text-muted-foreground text-xs'>
            {t('dashboard.entities.accounts')}
          </span>
          <span className='mt-1 font-medium'>{data.managed_account_count}</span>
        </Link>
        <Link
          className='border-border hover:bg-muted/70 grid rounded-lg border px-3 py-2.5 transition-colors'
          search={{
            auth: [],
            health: [],
            management: [],
            online: [],
            statistics: [],
          }}
          to='/sites'
        >
          <span className='text-muted-foreground text-xs'>
            {t('dashboard.entities.instances')}
          </span>
          <span className='mt-1 font-medium'>
            {data.instance_count == null || data.online_instance_count == null
              ? t('data.unavailableValue')
              : t('dashboard.entities.instanceValue', {
                  online: data.online_instance_count,
                  total: data.instance_count,
                })}
          </span>
        </Link>
      </div>
      <section
        aria-label={t('dashboard.entities.resourceCompleteness')}
        className='grid gap-3 border-t pt-3'
      >
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='flex flex-wrap items-center gap-2'>
            <span className='text-sm font-medium'>
              {t('dashboard.entities.resourceCompleteness')}
            </span>
            <DataStatusBadge status={data.resource_data_status} />
          </div>
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
        {data.resource_reason && (
          <p className='text-muted-foreground text-sm'>
            {translateMessageRef(data.resource_reason)}
          </p>
        )}
        {data.resource_stale_site_ids.length > 0 && (
          <StaleSiteLinks
            ids={data.resource_stale_site_ids}
            label={t('dashboard.entities.resourceStaleSites', { ids: '' })}
          />
        )}
      </section>
      {data.realtime_reason && (
        <p className='text-muted-foreground text-sm'>
          {translateMessageRef(data.realtime_reason)}
        </p>
      )}
      {data.stale_site_ids.length > 0 && (
        <StaleSiteLinks
          ids={data.stale_site_ids}
          label={t('dashboard.realtime.staleSites', { ids: '' })}
        />
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
        render={<Link search={search} to='/statistics/global' />}
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

function rankingSearch(
  item: DashboardRankingItem,
  metric: DashboardTopMetric
): StatisticsSearch {
  const start = dayjs().tz(BEIJING_TIMEZONE).startOf('day')
  const end = start.add(1, 'day')
  let siteIds: string[] = []
  if (item.dimension_type === 'site') {
    siteIds = [item.dimension_id]
  } else if (item.site_id) {
    siteIds = [item.site_id]
  }
  return buildStatisticsSearch({
    channelKeys: item.dimension_type === 'channel' ? [item.dimension_id] : [],
    customerIds: item.dimension_type === 'customer' ? [item.dimension_id] : [],
    display: 'quota',
    end: end.unix(),
    granularity: 'day',
    metric,
    models: item.dimension_type === 'model' ? [item.dimension_id] : [],
    order: 'desc',
    page: 1,
    pageSize: 20,
    siteIds,
    sort: metric,
    start: start.unix(),
    view: 'table',
  })
}

function RankingDetailLink({
  item,
  metric,
}: {
  item: DashboardRankingItem
  metric: DashboardTopMetric
}) {
  const { t } = useTranslation()
  const content = (
    <>
      <span className='font-medium break-words'>{item.dimension_name}</span>
      <span className='sr-only'>: {t('dashboard.ranking.openDetail')}</span>
    </>
  )
  const search = rankingSearch(item, metric)
  switch (item.dimension_type) {
    case 'site':
      return (
        <Link
          className='hover:underline'
          search={search}
          to='/statistics/sites'
        >
          {content}
        </Link>
      )
    case 'customer':
      return (
        <Link
          className='hover:underline'
          search={search}
          to='/statistics/customers'
        >
          {content}
        </Link>
      )
    case 'model':
      return (
        <Link
          className='hover:underline'
          search={search}
          to='/statistics/models'
        >
          {content}
        </Link>
      )
    case 'channel':
      return (
        <Link
          className='hover:underline'
          search={search}
          to='/statistics/channels'
        >
          {content}
        </Link>
      )
  }
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
              <TabsTrigger
                aria-controls='dashboard-ranking-panel'
                id={`dashboard-ranking-tab-${value}`}
                key={value}
                value={value}
              >
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
            <SelectControl
              className='w-20'
              onChange={(event) => onLimitChange(Number(event.target.value))}
              size='sm'
              value={limit}
            >
              <option value={5}>
                {t('dashboard.ranking.limitOption', { count: 5 })}
              </option>
              <option value={10}>
                {t('dashboard.ranking.limitOption', { count: 10 })}
              </option>
              <option value={20}>
                {t('dashboard.ranking.limitOption', { count: 20 })}
              </option>
            </SelectControl>
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
                <RankingDetailLink item={item} metric={metric} />
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
  const problemSites = data.sites.filter(isDashboardProblemSite)
  let problemSiteContent: ReactNode
  if (data.sites.length === 0) {
    problemSiteContent = (
      <p className='text-muted-foreground text-sm'>
        {t('dashboard.health.noSites')}
      </p>
    )
  } else if (problemSites.length === 0) {
    problemSiteContent = (
      <p className='text-muted-foreground text-sm'>
        {t('dashboard.health.allSitesHealthy')}
      </p>
    )
  } else {
    problemSiteContent = (
      <div className='grid gap-2 sm:grid-cols-2'>
        {problemSites.slice(0, 6).map((site) => (
          <Link
            className='border-border hover:bg-muted/50 grid min-w-0 gap-2 rounded-md border p-3 transition-colors'
            key={site.site_id}
            params={{ siteId: site.site_id }}
            to='/sites/$siteId'
          >
            <span className='font-medium break-words'>{site.site_name}</span>
            <div className='flex flex-wrap gap-1'>
              <Badge variant='neutral'>
                {t(dynamicI18nKey('site', `site.online.${site.online_status}`))}
              </Badge>
              <Badge variant='neutral'>
                {t(dynamicI18nKey('site', `site.auth.${site.auth_status}`))}
              </Badge>
              <Badge variant='neutral'>
                {t(dynamicI18nKey('site', `site.health.${site.health_status}`))}
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
    )
  }
  return (
    <div className='grid min-w-0 gap-5'>
      <div>
        <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
          <h3 className='font-medium'>{t('dashboard.health.problemSites')}</h3>
          <Button
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
            size='sm'
            variant='ghost'
          >
            {t('dashboard.openSites')}
          </Button>
        </div>
        {problemSiteContent}
      </div>
      <CompletenessAlert completeness={data.completeness} />
      <div className='grid gap-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h3 className='font-medium'>{t('dashboard.health.latestAlerts')}</h3>
          <Button
            render={
              <Link
                search={{
                  level: [],
                  ruleCategory: [],
                  ruleLevel: [],
                  status: [],
                  targetType: [],
                }}
                to='/alerts'
              />
            }
            size='sm'
            variant='ghost'
          >
            {t('dashboard.openAlerts')}
          </Button>
        </div>
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
                  <Link
                    aria-label={t('alerts.detail.open')}
                    className='font-medium break-words hover:underline'
                    search={{
                      alertId: alert.id,
                      level: [],
                      ruleCategory: [],
                      ruleLevel: [],
                      status: [],
                      targetType: [],
                    }}
                    to='/alerts'
                  >
                    {alert.target_name}
                  </Link>
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
    enabled: topType === 'site',
    placeholderData: keepPreviousData,
    queryFn: () => getDashboardTop('site', topMetric, topLimit),
    queryKey: dashboardKeys.top('site', topMetric, topLimit),
    staleTime: 5 * 60_000,
  })
  const customerTopQuery = useQuery({
    enabled: topType === 'customer',
    placeholderData: keepPreviousData,
    queryFn: () => getDashboardTop('customer', topMetric, topLimit),
    queryKey: dashboardKeys.top('customer', topMetric, topLimit),
    staleTime: 5 * 60_000,
  })
  const modelTopQuery = useQuery({
    enabled: topType === 'model',
    placeholderData: keepPreviousData,
    queryFn: () => getDashboardTop('model', topMetric, topLimit),
    queryKey: dashboardKeys.top('model', topMetric, topLimit),
    staleTime: 5 * 60_000,
  })
  const channelTopQuery = useQuery({
    enabled: topType === 'channel',
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
      actions={
        <>
          <Button
            render={
              <Link
                search={{
                  level: [],
                  ruleCategory: [],
                  ruleLevel: [],
                  status: [],
                  targetType: [],
                }}
                to='/alerts'
              />
            }
            variant='outline'
          >
            {t('dashboard.openAlerts')}
          </Button>
          <Button
            render={
              <Link
                search={buildStatisticsSearch({})}
                to='/statistics/global'
              />
            }
          >
            {t('dashboard.openStatistics')}
          </Button>
        </>
      }
      description={t('dashboard.description')}
      fixedContent
      title={t('dashboard.title')}
    >
      <div
        aria-label={t('dashboard.scrollArea')}
        className='h-full min-h-0 overflow-y-auto pr-1'
        role='region'
        tabIndex={0}
      >
        <div className='grid min-w-0 gap-4 pb-1 lg:grid-cols-12'>
          <DashboardPanel
            className='lg:col-span-12'
            icon={Analytics01Icon}
            id='today'
            state={summaryState}
            title={t('dashboard.section.today')}
          >
            {summaryQuery.data && <TodayOperations data={summaryQuery.data} />}
          </DashboardPanel>
          <DashboardPanel
            className='lg:col-span-8'
            empty={trendQuery.data?.length === 0}
            icon={Chart01Icon}
            id='trend'
            state={trendState}
            title={t('dashboard.section.trend')}
          >
            {trendQuery.data && <ThirtyDayTrend data={trendQuery.data} />}
          </DashboardPanel>
          <DashboardPanel
            className='lg:col-span-4'
            icon={Pulse01Icon}
            id='realtime'
            state={summaryState}
            title={t('dashboard.section.realtime')}
          >
            {summaryQuery.data && (
              <RealtimeThroughput data={summaryQuery.data} />
            )}
          </DashboardPanel>
          <DashboardPanel
            className='lg:col-span-7'
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
            className='lg:col-span-5'
            icon={Alert02Icon}
            id='health'
            state={healthState}
            title={t('dashboard.section.health')}
          >
            {healthQuery.data && (
              <div className='grid gap-4'>
                <OperationalAttention data={healthQuery.data} />
                <HealthAndCompleteness data={healthQuery.data} />
              </div>
            )}
          </DashboardPanel>
        </div>
      </div>
    </SectionPageLayout>
  )
}
