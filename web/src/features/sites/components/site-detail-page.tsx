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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useEffect, useState, type ReactNode } from 'react'
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
import { DataTablePagination } from '@/components/ui/data-table-pagination'
import { buildChannelInventorySearch } from '@/features/channel-inventory/search'
import { entityDetailFailure } from '@/features/entity-detail-query-state'
import { buildFinancialOperationsSearch } from '@/features/financial-operations/search'
import { buildLogSearch } from '@/features/logs/search'
import { buildModelCatalogSearch } from '@/features/model-catalog/search'
import { buildPerformanceHistorySearch } from '@/features/performance-history/search'
import { buildPricingGroupSearch } from '@/features/pricing-groups/search'
import { buildRankingSearch } from '@/features/rankings/search'
import { buildStatisticsSearch } from '@/features/statistics/search'
import { buildSubscriptionPlanSearch } from '@/features/subscription-plans/search'
import { buildSystemTaskSearch } from '@/features/system-tasks/search'
import { buildUpstreamTaskSearch } from '@/features/upstream-tasks/search'
import { buildUserInventorySearch } from '@/features/user-inventory/search'
import { useRetainedQueryData } from '@/hooks/use-retained-query-data'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { isRetryableApiError } from '@/lib/api'
import { isIdString, parseIdString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import {
  formatDecimalDisplayValue,
  formatDisplayValue,
} from '@/lib/display-value'
import { useAuthStore } from '@/stores/auth-store'

import { getSite, getSitePerformance, listSiteCollectionRuns } from '../api'
import { collectionTaskCatalog } from '../constants'
import { siteKeys } from '../query-keys'
import {
  formatPerformanceLatency,
  formatAverageRate,
  formatAverageTpm,
  formatPerformanceSuccessRate,
  formatPerformanceThroughput,
  formatPercentValue,
  sitePerformanceDashboardSummary,
} from '../site-card-metrics'
import type { SiteDetail, SitePerformanceSummary } from '../types'
import { SiteActions, type SiteAction } from './site-actions'
import { SiteDialogs, type SiteDialogState } from './site-dialogs'

interface SiteDetailPageProps {
  onDeleted: () => void
  siteId: string
}

function TimestampValue({ timestamp }: { timestamp: number | null }) {
  if (timestamp == null) return <span>-</span>
  return <span>{fromUnixSeconds(timestamp).format('YYYY-MM-DD HH:mm:ss')}</span>
}

function PercentValue({ value }: { value: number | null }) {
  const { t } = useTranslation()
  return <span>{formatPercentValue(value, t('data.unavailableValue'))}</span>
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
      <div className='grid gap-3 lg:grid-cols-[minmax(0,1.45fr)_minmax(0,1fr)]'>
        <div className='grid gap-2'>
          <h3 className='text-sm font-semibold'>{t('site.todayUsage')}</h3>
          <dl className='border-border [&>div]:border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 lg:grid-cols-3 [&>div]:border-b sm:[&>div]:border-r lg:[&>div:nth-child(3n)]:border-r-0'>
            <MetricCell label={t('site.dashboard.last24HoursCount')}>
              <MetricValue nullLabel='0' value={site.today.request_count} />
            </MetricCell>
            <MetricCell label={t('site.dashboard.last24HoursQuota')}>
              <QuotaAmount
                nullLabel='0'
                quota={site.today.quota}
                rate={site.rate}
              />
            </MetricCell>
            <MetricCell label={t('site.dashboard.last24HoursTokens')}>
              <MetricValue nullLabel='0' value={site.today.token_used} />
            </MetricCell>
            <MetricCell label={t('site.averageRpm')}>
              <span title={site.today.avg_rpm ?? undefined}>
                {formatAverageRate(site.today.avg_rpm)}
              </span>
            </MetricCell>
            <MetricCell label={t('site.averageTpm')}>
              <span title={site.today.avg_tpm ?? undefined}>
                {formatAverageTpm(site.today.avg_tpm)}
              </span>
            </MetricCell>
            <MetricCell label={t('site.activeUsers')}>
              <MetricValue nullLabel='0' value={site.today.active_users} />
            </MetricCell>
          </dl>
        </div>
        <div className='grid gap-2'>
          <h3 className='text-sm font-semibold'>{t('site.resources')}</h3>
          <dl className='border-border [&>div]:border-border grid h-full overflow-hidden rounded-lg border sm:grid-cols-2 [&>div]:border-b sm:[&>div:nth-child(2n)]:border-r-0'>
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
            <MetricCell label={t('instance.summary')}>
              <span>
                {site.resource.instance_count == null
                  ? '-'
                  : site.resource.instance_count}
              </span>
            </MetricCell>
            <MetricCell label={t('site.resourceStatus')}>
              <DataStatusBadge status={site.resource.data_status} />
            </MetricCell>
          </dl>
        </div>
      </div>
      <div className='flex flex-wrap items-center justify-between gap-3 border-b pb-4'>
        <div className='flex items-center gap-2'>
          <span className='text-muted-foreground text-sm'>
            {t('site.last24HoursDataStatus')}
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
  const performanceModels = performance?.models ?? []
  const [modelPage, setModelPage] = useState(1)
  const [modelPageSize, setModelPageSize] = useState(10)
  useEffect(() => setModelPage(1), [range])
  const performanceSummary = sitePerformanceDashboardSummary(performanceModels)
  const visibleModels = performanceModels.slice(
    (modelPage - 1) * modelPageSize,
    modelPage * modelPageSize
  )
  const unavailableValue = t('data.unavailableValue')
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
      <div className='grid gap-3'>
        <div className='grid gap-2 sm:grid-cols-3'>
          <div className='border-border flex items-center justify-between gap-3 rounded-full border px-4 py-3'>
            <span className='text-muted-foreground text-sm'>
              {t('site.performance.successRate')}
            </span>
            <span className='text-lg font-semibold tabular-nums'>
              {formatPerformanceSuccessRate(
                performanceSummary.successRate,
                unavailableValue
              )}
            </span>
          </div>
          <div className='border-border flex items-center justify-between gap-3 rounded-full border px-4 py-3'>
            <span className='text-muted-foreground text-sm'>
              {t('site.performance.avgLatency')}
            </span>
            <span className='text-lg font-semibold tabular-nums'>
              {formatPerformanceLatency(
                performanceSummary.avgLatencyMs,
                unavailableValue
              )}
            </span>
          </div>
          <div className='border-border flex items-center justify-between gap-3 rounded-full border px-4 py-3'>
            <span className='text-muted-foreground text-sm'>
              {t('site.performance.avgTps')}
            </span>
            <span className='text-lg font-semibold tabular-nums'>
              {formatPerformanceThroughput(
                performanceSummary.throughput,
                unavailableValue
              )}
            </span>
          </div>
        </div>
        <div className='grid gap-3'>
          <h3 className='text-sm font-semibold'>
            {t('site.performance.models')}
          </h3>
          <div className='overflow-hidden rounded-lg border'>
            <div className='bg-muted/40 hidden grid-cols-4 gap-3 border-b px-4 py-2 text-xs font-medium sm:grid'>
              <span>{t('site.performance.model')}</span>
              <span className='text-right'>
                {t('site.performance.successRate')}
              </span>
              <span className='text-right'>
                {t('site.performance.avgLatency')}
              </span>
              <span className='text-right'>{t('site.performance.avgTps')}</span>
            </div>
            {performanceModels.length === 0 ? (
              <p className='text-muted-foreground px-4 py-6 text-center text-sm'>
                {t('site.performance.unavailable')}
              </p>
            ) : (
              <dl className='divide-y'>
                {visibleModels.map((model) => (
                  <div
                    className='grid grid-cols-2 gap-x-3 gap-y-2 px-4 py-3 sm:grid-cols-4'
                    key={model.model_name}
                  >
                    <dt
                      className='col-span-2 truncate text-sm font-medium sm:col-span-1'
                      title={model.model_name}
                    >
                      {model.model_name}
                    </dt>
                    <dd className='text-sm sm:text-right'>
                      <span className='text-muted-foreground sm:hidden'>
                        {t('site.performance.successRate')}:{' '}
                      </span>
                      {formatPerformanceSuccessRate(
                        model.success_rate,
                        unavailableValue
                      )}
                    </dd>
                    <dd className='text-sm sm:text-right'>
                      <span className='text-muted-foreground sm:hidden'>
                        {t('site.performance.avgLatency')}:{' '}
                      </span>
                      {formatPerformanceLatency(
                        model.avg_latency_ms,
                        unavailableValue
                      )}
                    </dd>
                    <dd className='text-sm sm:text-right'>
                      <span className='text-muted-foreground sm:hidden'>
                        {t('site.performance.avgTps')}:{' '}
                      </span>
                      {formatPerformanceThroughput(
                        model.avg_tps,
                        unavailableValue
                      )}
                    </dd>
                  </div>
                ))}
              </dl>
            )}
          </div>
          {performanceModels.length > 0 && (
            <DataTablePagination
              onPageChange={setModelPage}
              onPageSizeChange={(pageSize) => {
                setModelPageSize(pageSize)
                setModelPage(1)
              }}
              page={modelPage}
              pageSize={modelPageSize}
              total={performanceModels.length}
            />
          )}
        </div>
      </div>
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
              className='min-h-10 px-2.5 sm:min-h-8'
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
            {formatDecimalDisplayValue(site.rate.quota_per_unit)}
          </p>
        </div>
        <div>
          <p className='text-muted-foreground text-xs'>
            {t('site.rate.usdExchangeRate')}
          </p>
          <p className='mt-1 font-medium'>
            {formatDecimalDisplayValue(site.rate.usd_exchange_rate)}
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

const relatedLinkClass =
  'border-border hover:bg-muted flex min-h-10 items-center gap-2 rounded-md border px-3 py-2 text-sm font-medium'

function collectionRunBadgeVariant(
  status: 'failed' | 'pending' | 'running' | 'success'
): 'destructive' | 'neutral' | 'primary' | 'success' {
  if (status === 'success') return 'success'
  if (status === 'failed') return 'destructive'
  if (status === 'running') return 'primary'
  return 'neutral'
}

function SiteRelatedPages({ siteId }: { siteId: string }) {
  const { t } = useTranslation()
  return (
    <section aria-labelledby='site-related-pages-title' className='grid gap-3'>
      <div>
        <h2 className='text-lg font-semibold' id='site-related-pages-title'>
          {t('site.related.title')}
        </h2>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t('site.related.description')}
        </p>
      </div>
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
        <nav
          aria-label={t('site.related.operations')}
          className='border-border grid content-start gap-2 rounded-lg border p-3'
        >
          <h3 className='text-sm font-semibold'>
            {t('site.related.operations')}
          </h3>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildFinancialOperationsSearch({})}
            to='/sites/$siteId/financial-operations'
          >
            <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
            {t('site.actions.financialOperations')}
          </Link>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildPerformanceHistorySearch({})}
            to='/sites/$siteId/performance-history'
          >
            <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
            {t('site.actions.performanceHistory')}
          </Link>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildStatisticsSearch({})}
            to='/sites/$siteId/stats'
          >
            <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
            {t('site.actions.stats')}
          </Link>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildRankingSearch({})}
            to='/sites/$siteId/rankings'
          >
            <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
            {t('site.actions.rankings')}
          </Link>
        </nav>

        <nav
          aria-label={t('site.related.resources')}
          className='border-border grid content-start gap-2 rounded-lg border p-3'
        >
          <h3 className='text-sm font-semibold'>
            {t('site.related.resources')}
          </h3>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildUserInventorySearch({})}
            to='/sites/$siteId/user-inventory'
          >
            <HugeiconsIcon icon={UserGroupIcon} strokeWidth={2} />
            {t('site.actions.userInventory')}
          </Link>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildChannelInventorySearch({})}
            to='/sites/$siteId/channel-inventory'
          >
            <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
            {t('site.actions.channelInventory')}
          </Link>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildModelCatalogSearch({})}
            to='/sites/$siteId/model-catalog'
          >
            <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
            {t('site.actions.modelCatalog')}
          </Link>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildPricingGroupSearch({})}
            to='/sites/$siteId/pricing-groups'
          >
            <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
            {t('site.actions.pricingGroups')}
          </Link>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildSubscriptionPlanSearch({})}
            to='/sites/$siteId/subscription-plans'
          >
            <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
            {t('site.actions.subscriptionPlans')}
          </Link>
        </nav>

        <nav
          aria-label={t('site.related.records')}
          className='border-border grid content-start gap-2 rounded-lg border p-3'
        >
          <h3 className='text-sm font-semibold'>{t('site.related.records')}</h3>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildLogSearch({})}
            to='/sites/$siteId/logs'
          >
            <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
            {t('site.actions.logs')}
          </Link>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildUpstreamTaskSearch({})}
            to='/sites/$siteId/upstream-tasks'
          >
            <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
            {t('site.actions.upstreamTasks')}
          </Link>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            search={buildSystemTaskSearch({})}
            to='/sites/$siteId/system-tasks'
          >
            <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
            {t('site.actions.systemTasks')}
          </Link>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            to='/sites/$siteId/collection-runs'
          >
            <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
            {t('site.actions.collectionRuns')}
          </Link>
        </nav>

        <nav
          aria-label={t('site.related.infrastructure')}
          className='border-border grid content-start gap-2 rounded-lg border p-3'
        >
          <h3 className='text-sm font-semibold'>
            {t('site.related.infrastructure')}
          </h3>
          <Link
            className={relatedLinkClass}
            params={{ siteId }}
            to='/sites/$siteId/status'
          >
            <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
            {t('site.instanceStatus')}
          </Link>
        </nav>
      </div>
    </section>
  )
}

function RecentCollectionActivity({ siteId }: { siteId: string }) {
  const { t } = useTranslation()
  const params = { p: 1, page_size: 3 }
  const runsQuery = useQuery({
    queryFn: () => listSiteCollectionRuns(parseIdString(siteId), params),
    queryKey: siteKeys.runs(siteId, params),
    staleTime: 5_000,
  })
  const runs = runsQuery.data?.items ?? []
  let content: ReactNode
  if (runsQuery.isPending) {
    content = <LoadingState message={t('site.collectionRecent.loading')} />
  } else if (runsQuery.isError) {
    content = (
      <p className='text-destructive text-sm' role='alert'>
        {t('site.collectionRecent.loadError')}
      </p>
    )
  } else if (runs.length === 0) {
    content = (
      <p className='text-muted-foreground text-sm'>{t('collection.empty')}</p>
    )
  } else {
    content = (
      <div className='border-border divide-border divide-y rounded-lg border'>
        {runs.map((run) => (
          <article
            className='flex flex-wrap items-center justify-between gap-3 px-3 py-2.5'
            key={run.id}
          >
            <div className='min-w-0'>
              <p className='font-medium'>
                {t(dynamicI18nKey('site', `collection.task.${run.task_type}`))}
              </p>
              <p className='text-muted-foreground truncate text-xs'>
                {t(
                  dynamicI18nKey(
                    'site',
                    collectionTaskCatalog[run.task_type].purposeKey
                  )
                )}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t('collection.taskId')}: {run.id}
              </p>
            </div>
            <div className='flex items-center gap-3'>
              <span className='text-muted-foreground text-xs'>
                {fromUnixSeconds(run.created_at).format('MM-DD HH:mm:ss')}
              </span>
              <Badge variant={collectionRunBadgeVariant(run.status)}>
                {t(dynamicI18nKey('site', `collection.status.${run.status}`))}
              </Badge>
            </div>
          </article>
        ))}
      </div>
    )
  }
  return (
    <section
      aria-labelledby='site-recent-collection-title'
      className='grid gap-3'
    >
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h2
            className='text-lg font-semibold'
            id='site-recent-collection-title'
          >
            {t('site.collectionRecent.title')}
          </h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('site.collectionRecent.description', {
              total: runsQuery.data?.total ?? 0,
            })}
          </p>
        </div>
        <Link
          className={relatedLinkClass}
          params={{ siteId }}
          to='/sites/$siteId/collection-runs'
        >
          <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
          {t('site.actions.collectionRuns')}
        </Link>
      </div>
      {content}
    </section>
  )
}

export function SiteDetailPage({ onDeleted, siteId }: SiteDetailPageProps) {
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
    retry: (failureCount, error) =>
      failureCount < 2 && isRetryableApiError(error),
    refetchInterval: (query) =>
      query.state.data?.statistics_status === 'backfilling' ? 5_000 : 60_000,
    staleTime: 30_000,
  })
  const performanceQuery = useQuery({
    enabled: validSiteId,
    queryFn: () => getSitePerformance(parseIdString(siteId), performanceRange),
    queryKey: siteKeys.performance(siteId, performanceRange),
    staleTime: 60_000,
  })
  const site = useRetainedQueryData(
    detailQuery.data,
    detailQuery.isError,
    `site-detail:${siteId}`
  )

  const retry = () => {
    void detailQuery.refetch()
    void performanceQuery.refetch()
  }
  const invalidate = (action: SiteAction) => {
    void queryClient.invalidateQueries({ queryKey: siteKeys.all })
    if (action === 'delete') onDeleted()
  }

  const actions =
    site && isAdmin ? (
      <SiteActions
        onAction={(action, selectedSite) =>
          setDialogState({ action, site: selectedSite })
        }
        site={site}
      />
    ) : undefined

  let detailContent: ReactNode
  if (!validSiteId || (detailQuery.isError && !site)) {
    const failure = entityDetailFailure(
      validSiteId,
      detailQuery.error,
      'site.detail.loadErrorDescription',
      'site.detail.invalidId'
    )
    detailContent = (
      <ErrorState
        description={t(dynamicI18nKey('site', failure.descriptionKey))}
        onRetry={failure.retryable ? retry : undefined}
        title={
          failure.kind === 'invalid-id'
            ? t('site.detail.invalidId')
            : t('site.detail.loadError')
        }
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
          <CompletenessAlert
            completeness={site.completeness}
            scopeDescription={t('completeness.siteScopeDescription')}
          />
          <BackfillProgress backfill={site.backfill} />
        </div>
        <SiteMetadata site={site} />
        <RecentCollectionActivity siteId={siteId} />
        <SiteRelatedPages siteId={siteId} />
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
      </div>

      <SiteDialogs
        onClose={() => setDialogState(null)}
        onSaved={invalidate}
        state={dialogState}
      />
    </SectionPageLayout>
  )
}
