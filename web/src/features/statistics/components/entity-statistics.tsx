import {
  Chart01Icon,
  FileExportIcon,
  TableIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQueryClient } from '@tanstack/react-query'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import type { ILineChartSpec } from '@visactor/react-vchart'
import { lazy, Suspense, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CompletenessAlert } from '@/components/data/completeness-alert'
import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { useTheme } from '@/context/theme-provider'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { calculateCrossSiteQuotaAmount, formatDecimal } from '@/lib/amount'
import { getApiErrorTranslationKey } from '@/lib/api'
import type { IdString } from '@/lib/api-types'
import {
  BEIJING_TIMEZONE,
  dayjs,
  formatBeijingTimestamp,
  fromUnixSeconds,
} from '@/lib/dayjs'
import { translateMessageRef } from '@/lib/message-ref'

import { createStatisticsExport } from '../api'
import { buildTrendChartModel, type TrendChartDatum } from '../chart-data'
import { buildEntityExportRequest } from '../export-request'
import { statisticsKeys } from '../query-keys'
import { defaultStatisticsRange } from '../search'
import type {
  AccountStatisticsBreakdown,
  SiteQuotaBreakdown,
  StatisticsBreakdownBase,
  StatisticsDisplay,
  StatisticsExportFormat,
  StatisticsExportJobItem,
  StatisticsGranularity,
  StatisticsMetric,
  StatisticsResponse,
  StatisticsSearch,
  StatisticsScope,
} from '../types'
import { ExportDialog } from './export-dialog'
import { ExportTaskSheet } from './export-task-sheet'

const LazyVChart = lazy(() =>
  import('@visactor/react-vchart').then((module) => ({
    default: module.VChart,
  }))
)

export function ActiveUsersValue({
  compact = false,
  scope,
  value,
}: {
  compact?: boolean
  scope: StatisticsScope
  value: StatisticsBreakdownBase['active_users']
}) {
  const { t } = useTranslation()
  return (
    <MetricValue
      compact={compact}
      nullLabel={
        scope === 'account'
          ? t('statistics.metric.active_users_unavailable')
          : undefined
      }
      value={value}
    />
  )
}

function crossSiteAmount(siteBreakdown: SiteQuotaBreakdown[]) {
  return calculateCrossSiteQuotaAmount(
    siteBreakdown.map((site) => ({
      quota: site.quota,
      rate: {
        quota_per_unit: site.quota_per_unit,
        source: site.rate_source,
        updated_at: site.rate_updated_at,
        usd_exchange_rate: site.usd_exchange_rate,
      },
      siteId: site.site_id,
    }))
  )
}

export function AmountValue({
  display,
  siteBreakdown,
}: {
  display: StatisticsDisplay
  siteBreakdown: SiteQuotaBreakdown[]
}) {
  const { t } = useTranslation()
  const amount = useMemo(() => crossSiteAmount(siteBreakdown), [siteBreakdown])
  if (display === 'quota') {
    return amount.quota == null ? (
      <span>{t('data.unavailableValue')}</span>
    ) : (
      <span title={amount.quota.toFixed(0)}>{amount.quota.toFixed(0)}</span>
    )
  }
  if (amount.status !== 'available') {
    return (
      <span className='text-warning-foreground'>
        {t(
          dynamicI18nKey(
            'statistics',
            amount.status === 'partial_rate_unavailable'
              ? 'amount.partialRateUnavailable'
              : 'amount.rateUnavailable'
          )
        )}
      </span>
    )
  }
  const value = display === 'usd' ? amount.amountUsd : amount.amountCny
  return (
    <span title={formatDecimal(value, 6) ?? undefined}>
      {t(
        dynamicI18nKey(
          'statistics',
          display === 'usd'
            ? 'statistics.amount.usdValue'
            : 'statistics.amount.cnyValue'
        ),
        { value: formatDecimal(value) }
      )}
    </span>
  )
}

function inputType(granularity: StatisticsGranularity) {
  if (granularity === 'hour') return 'datetime-local'
  if (granularity === 'month') return 'month'
  if (granularity === 'year') return 'number'
  return 'date'
}

function inputValue(timestamp: number, granularity: StatisticsGranularity) {
  const value = fromUnixSeconds(timestamp)
  if (granularity === 'hour') return value.format('YYYY-MM-DDTHH:00')
  if (granularity === 'month') return value.format('YYYY-MM')
  if (granularity === 'year') return value.format('YYYY')
  return value.format('YYYY-MM-DD')
}

function parseInput(value: string, granularity: StatisticsGranularity) {
  let format = 'YYYY-MM-DD'
  if (granularity === 'hour') format = 'YYYY-MM-DDTHH:mm'
  else if (granularity === 'month') format = 'YYYY-MM'
  else if (granularity === 'year') format = 'YYYY'
  const parsed = dayjs.tz(value, format, BEIJING_TIMEZONE)
  return parsed.isValid() ? parsed.startOf(granularity).unix() : null
}

export function StatisticsToolbar({
  exportDisabled,
  filterAction,
  onExportOpen,
  onSearchChange,
  search,
}: {
  exportDisabled: boolean
  filterAction?: ReactNode
  onExportOpen: () => void
  onSearchChange: (changes: Partial<StatisticsSearch>) => void
  search: StatisticsSearch
}) {
  const { t } = useTranslation()
  const granularities: StatisticsGranularity[] = [
    'hour',
    'day',
    'month',
    'year',
  ]
  return (
    <section
      aria-label={t('statistics.filters')}
      className='grid gap-4 border-b pb-5'
    >
      <div className='flex flex-wrap items-center gap-2' role='group'>
        {granularities.map((granularity) => (
          <Button
            aria-pressed={search.granularity === granularity}
            className='min-h-10 min-w-10'
            key={granularity}
            onClick={() =>
              onSearchChange({
                ...defaultStatisticsRange(granularity),
                granularity,
                page: 1,
              })
            }
            size='sm'
            variant={
              search.granularity === granularity ? 'secondary' : 'outline'
            }
          >
            {t(
              dynamicI18nKey(
                'statistics',
                `statistics.granularity.${granularity}`
              )
            )}
          </Button>
        ))}
      </div>
      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <label className='grid gap-1 text-sm'>
          <span>{t('statistics.start')}</span>
          <Input
            max={inputValue(search.end, search.granularity)}
            onChange={(event) => {
              const start = parseInput(event.target.value, search.granularity)
              if (start != null && start < search.end)
                onSearchChange({ page: 1, start })
            }}
            type={inputType(search.granularity)}
            value={inputValue(search.start, search.granularity)}
          />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('statistics.end')}</span>
          <Input
            min={inputValue(search.start, search.granularity)}
            onChange={(event) => {
              const end = parseInput(event.target.value, search.granularity)
              if (end != null && end > search.start)
                onSearchChange({ end, page: 1 })
            }}
            type={inputType(search.granularity)}
            value={inputValue(search.end, search.granularity)}
          />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('statistics.metric')}</span>
          <Select
            onChange={(event) =>
              onSearchChange({
                metric: event.target.value as StatisticsMetric,
                page: 1,
              })
            }
            value={search.metric}
          >
            {(
              ['request_count', 'quota', 'token_used', 'active_users'] as const
            ).map((metric) => (
              <option key={metric} value={metric}>
                {t(dynamicI18nKey('statistics', `statistics.metric.${metric}`))}
              </option>
            ))}
          </Select>
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('statistics.amountDisplay')}</span>
          <Select
            disabled={search.metric !== 'quota'}
            onChange={(event) =>
              onSearchChange({
                display: event.target.value as StatisticsDisplay,
              })
            }
            value={search.display}
          >
            {(['quota', 'usd', 'cny'] as const).map((display) => (
              <option key={display} value={display}>
                {t(
                  dynamicI18nKey('statistics', `statistics.display.${display}`)
                )}
              </option>
            ))}
          </Select>
        </label>
      </div>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <div
            className='border-border flex w-fit rounded-md border p-0.5'
            role='group'
          >
            <Button
              aria-label={t('statistics.chartView')}
              aria-pressed={search.view === 'chart'}
              onClick={() => onSearchChange({ view: 'chart' })}
              size='icon'
              title={t('statistics.chartView')}
              variant={search.view === 'chart' ? 'secondary' : 'ghost'}
            >
              <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
            </Button>
            <Button
              aria-label={t('statistics.tableView')}
              aria-pressed={search.view === 'table'}
              onClick={() => onSearchChange({ view: 'table' })}
              size='icon'
              title={t('statistics.tableView')}
              variant={search.view === 'table' ? 'secondary' : 'ghost'}
            >
              <HugeiconsIcon icon={TableIcon} strokeWidth={2} />
            </Button>
          </div>
          {filterAction}
        </div>
        <Button
          disabled={exportDisabled}
          onClick={onExportOpen}
          variant='outline'
        >
          <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
          {t('statistics.export.open')}
        </Button>
      </div>
    </section>
  )
}

export function MetricTrendChart({
  data,
  search,
}: {
  data: StatisticsResponse['trend']
  search: StatisticsSearch
}) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const model = useMemo(
    () =>
      buildTrendChartModel(
        data,
        search.metric,
        search.display,
        search.granularity
      ),
    [data, search.display, search.granularity, search.metric]
  )
  const valueOf = (datum: unknown) => datum as TrendChartDatum | undefined
  const spec = useMemo<ILineChartSpec>(
    () => ({
      axes: [
        { orient: 'bottom', type: 'band' },
        { orient: 'left', type: 'linear' },
      ],
      data: [{ id: 'trend', values: model.values }],
      invalidType: 'break',
      line: {
        style: {
          lineDash: (datum) => (valueOf(datum)?.partial ? [6, 4] : []),
          lineWidth: 2,
        },
      },
      point: {
        style: {
          fillOpacity: (datum) => (valueOf(datum)?.partial ? 0 : 1),
          lineWidth: (datum) => (valueOf(datum)?.partial ? 2 : 1),
          size: 8,
        },
        visible: true,
      },
      theme: resolvedTheme === 'dark' ? 'dark' : 'light',
      tooltip: {
        activeType: 'dimension',
        dimension: {
          content: [
            {
              key: t('statistics.tooltip.raw'),
              value: (datum) =>
                valueOf(datum)?.rawValue ?? t('data.unavailableValue'),
            },
            {
              key: t('statistics.tooltip.displayValue'),
              value: (datum) =>
                valueOf(datum)?.exactValue ?? t('data.unavailableValue'),
            },
            {
              key: t('statistics.tooltip.dataStatus'),
              value: (datum) => {
                const status = valueOf(datum)?.data_status
                return status
                  ? t(dynamicI18nKey('data', `data.${status}`))
                  : t('data.unavailableValue')
              },
            },
            {
              key: t('statistics.tooltip.completeSites'),
              value: (datum) => {
                const point = valueOf(datum)
                return point
                  ? `${point.complete_site_count}/${point.expected_site_count}`
                  : ''
              },
            },
            {
              key: t('statistics.tooltip.final'),
              value: (datum) =>
                valueOf(datum)?.is_final ? t('common.yes') : t('common.no'),
            },
            {
              key: t('statistics.tooltip.asOf'),
              value: (datum) => {
                const asOf = valueOf(datum)?.as_of
                return asOf == null
                  ? t('data.unavailableValue')
                  : fromUnixSeconds(asOf).format('YYYY-MM-DD HH:mm:ss')
              },
            },
            {
              key: t('statistics.tooltip.reason'),
              value: (datum) => {
                const reason = valueOf(datum)?.reason
                return reason ? translateMessageRef(reason) : t('common.none')
              },
            },
            {
              key: t('statistics.tooltip.siteAmounts'),
              value: (datum) => {
                const sites = valueOf(datum)?.siteAmounts ?? []
                if (sites.length === 0) return t('common.none')
                return sites
                  .map((site) =>
                    t('statistics.tooltip.siteLine', {
                      cny: site.amountCny ?? t('data.unavailableValue'),
                      exchange:
                        site.usdExchangeRate ?? t('data.unavailableValue'),
                      id: site.siteId,
                      name: site.siteName,
                      quota: site.quota ?? t('data.unavailableValue'),
                      quotaPerUnit:
                        site.quotaPerUnit ?? t('data.unavailableValue'),
                      rateSource: t(
                        dynamicI18nKey(
                          'statistics',
                          `statistics.rateSource.${site.rateSource}`
                        )
                      ),
                      rateTime:
                        site.rateUpdatedAt == null
                          ? t('data.unavailableValue')
                          : fromUnixSeconds(site.rateUpdatedAt).format(
                              'YYYY-MM-DD HH:mm:ss'
                            ),
                      status: t(
                        dynamicI18nKey('data', `data.${site.dataStatus}`)
                      ),
                      usd: site.amountUsd ?? t('data.unavailableValue'),
                    })
                  )
                  .join('\n')
              },
            },
          ],
          title: { value: (datum) => valueOf(datum)?.label ?? '' },
        },
        renderMode: 'html',
        visible: true,
      },
      type: 'line',
      xField: 'label',
      yField: 'chartValue',
    }),
    [model.values, resolvedTheme, t]
  )
  if (data.length === 0) {
    return (
      <p className='text-muted-foreground border-t py-8 text-center text-sm'>
        {t('statistics.emptyTrend')}
      </p>
    )
  }
  const scaled = model.baseline !== '0' || model.scale !== '1'
  return (
    <figure className='grid min-w-0 gap-2'>
      <div
        aria-label={t('statistics.trendChart')}
        className='h-72 min-h-72 w-full min-w-0 overflow-hidden sm:h-80 sm:min-h-80'
        role='img'
      >
        <Suspense
          fallback={
            <div className='bg-muted h-full animate-pulse rounded-md' />
          }
        >
          <LazyVChart spec={spec} />
        </Suspense>
      </div>
      <figcaption className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs'>
        <span className='inline-flex items-center gap-2'>
          <span className='border-foreground block w-6 border-t border-dashed' />
          {t('statistics.chart.partialLegend')}
        </span>
        {scaled && (
          <span data-testid='statistics-chart-scale'>
            {t('statistics.chart.scale', {
              baseline: model.baseline,
              scale: model.scale,
            })}
          </span>
        )}
      </figcaption>
      <ol
        aria-label={t('statistics.chartExactValues')}
        className='sr-only'
        data-testid='statistics-chart-exact-values'
      >
        {model.values.map((point) => (
          <li key={point.bucket_start}>
            {point.label}: {t('statistics.tooltip.raw')} {point.rawValue ?? '-'}
            ;{t('statistics.tooltip.displayValue')} {point.exactValue ?? '-'};
            {point.complete_site_count}/{point.expected_site_count};
            {point.is_final ? t('common.yes') : t('common.no')}
          </li>
        ))}
      </ol>
    </figure>
  )
}

export function StatisticsSummary({
  data,
  search,
}: {
  data: StatisticsResponse
  search: StatisticsSearch
}) {
  const { t } = useTranslation()
  let statusDescription = t('statistics.partialDescription')
  if (data.summary.data_status === 'missing') {
    statusDescription = t('statistics.missingDescription')
  } else if (data.summary.data_status === 'unavailable') {
    statusDescription = t('statistics.unavailableDescription')
  } else if (data.summary.data_status === 'paused') {
    statusDescription = t('statistics.pausedDescription')
  }
  return (
    <section aria-labelledby='statistics-summary-title' className='grid gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <h2 className='text-lg font-semibold' id='statistics-summary-title'>
          {t('statistics.summary')}
        </h2>
        <DataFreshness
          labelKey='statistics.asOf'
          timestamp={data.range.as_of}
        />
      </div>
      {(data.summary.is_partial || data.summary.data_status !== 'complete') && (
        <div
          className='border-warning/40 bg-warning/10 flex flex-wrap items-center justify-between gap-2 rounded-md border p-3'
          role='status'
        >
          <span className='text-sm'>{statusDescription}</span>
          <DataStatusBadge status={data.summary.data_status} />
        </div>
      )}
      <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-5 [&_dd]:leading-tight [&_dd]:break-all [&_dd]:tabular-nums [&>div]:min-w-0 [&>div]:border-r [&>div]:border-b'>
        <div className='p-4'>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.request_count')}
          </dt>
          <dd className='mt-1 text-xl font-semibold'>
            <MetricValue value={data.summary.request_count} />
          </dd>
        </div>
        <div className='p-4'>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.quota')}
          </dt>
          <dd className='mt-1 text-xl font-semibold'>
            <MetricValue value={data.summary.quota} />
          </dd>
        </div>
        <div className='p-4'>
          <dt className='text-muted-foreground text-xs'>
            {t(
              dynamicI18nKey(
                'statistics',
                `statistics.display.${search.display}`
              )
            )}
          </dt>
          <dd className='mt-1 text-lg font-semibold'>
            <AmountValue
              display={search.display}
              siteBreakdown={data.site_breakdown}
            />
          </dd>
        </div>
        <div className='p-4'>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.token_used')}
          </dt>
          <dd className='mt-1 text-xl font-semibold'>
            <MetricValue value={data.summary.token_used} />
          </dd>
        </div>
        <div className='p-4'>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.active_users')}
          </dt>
          <dd className='mt-1 text-xl font-semibold'>
            <ActiveUsersValue
              scope={data.scope}
              value={data.summary.active_users}
            />
          </dd>
        </div>
      </dl>
    </section>
  )
}

export function useStatisticsEmptyCopy(
  status: StatisticsResponse['summary']['data_status']
) {
  const { t } = useTranslation()
  if (status === 'missing') {
    return {
      description: t('statistics.emptyMissingDescription'),
      title: t('statistics.emptyMissing'),
    }
  }
  if (status === 'unavailable') {
    return {
      description: t('statistics.emptyUnavailableDescription'),
      title: t('statistics.emptyUnavailable'),
    }
  }
  if (status === 'paused') {
    return {
      description: t('statistics.emptyPausedDescription'),
      title: t('statistics.emptyPaused'),
    }
  }
  if (status !== 'complete') {
    return {
      description: t('statistics.emptyPartialDescription'),
      title: t('statistics.emptyPartial'),
    }
  }
  return {
    description: t('statistics.emptyDescription'),
    title: t('statistics.empty'),
  }
}

export function SiteBreakdownList({ sites }: { sites: SiteQuotaBreakdown[] }) {
  const { t } = useTranslation()
  if (sites.length === 0) {
    return <span className='text-muted-foreground'>{t('common.none')}</span>
  }
  return (
    <ul className='grid min-w-56 gap-2 text-xs'>
      {sites.map((site) => {
        const amount = crossSiteAmount([site])
        return (
          <li
            className='border-border grid gap-2 border-b pb-2'
            key={site.site_id}
          >
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <span className='font-medium'>{site.site_name}</span>
              <code className='text-muted-foreground'>
                {t('statistics.identity.id', { id: site.site_id })}
              </code>
            </div>
            <div className='flex flex-wrap items-center gap-2'>
              <DataStatusBadge status={site.data_status} />
              <span className='text-muted-foreground'>
                {t(
                  dynamicI18nKey(
                    'statistics',
                    `statistics.rateSource.${site.rate_source}`
                  )
                )}
              </span>
              <span className='text-muted-foreground'>
                {t('statistics.siteBreakdown.rateUpdatedAt', {
                  time:
                    site.rate_updated_at == null
                      ? t('data.unavailableValue')
                      : fromUnixSeconds(site.rate_updated_at).format(
                          'YYYY-MM-DD HH:mm:ss'
                        ),
                })}
              </span>
            </div>
            <dl className='grid grid-cols-2 gap-x-3 gap-y-1'>
              <div>
                <dt className='text-muted-foreground'>
                  {t('statistics.metric.quota')}
                </dt>
                <dd>
                  <MetricValue compact value={site.quota} />
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('statistics.siteBreakdown.quotaPerUnit')}
                </dt>
                <dd>{site.quota_per_unit ?? t('data.unavailableValue')}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('statistics.siteBreakdown.exchangeRate')}
                </dt>
                <dd>{site.usd_exchange_rate ?? t('data.unavailableValue')}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('statistics.display.usd')}
                </dt>
                <dd>
                  {formatDecimal(amount.amountUsd, 6) ??
                    t('data.unavailableValue')}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('statistics.display.cny')}
                </dt>
                <dd>
                  {formatDecimal(amount.amountCny, 6) ??
                    t('data.unavailableValue')}
                </dd>
              </div>
            </dl>
          </li>
        )
      })}
    </ul>
  )
}

function BreakdownMobileCard({
  item,
  scope,
  granularity,
}: {
  item: StatisticsBreakdownBase
  scope: 'site' | 'customer' | 'account'
  granularity: StatisticsGranularity
}) {
  const { t } = useTranslation()
  const account =
    scope === 'account' ? (item as AccountStatisticsBreakdown) : null
  return (
    <article className='border-border bg-card grid min-w-0 gap-4 rounded-lg border p-4'>
      <header className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
        <div className='min-w-0'>
          <h3 className='font-medium break-words'>{item.dimension_name}</h3>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('statistics.identity.bucket', {
              id: item.dimension_id,
              time: formatBeijingTimestamp(item.bucket_start, granularity),
            })}
          </p>
        </div>
        <DataStatusBadge status={item.data_status} />
      </header>
      {account && (
        <dl className='grid grid-cols-2 gap-3 text-sm'>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('statistics.account.siteIdentity')}
            </dt>
            <dd className='break-words'>
              {t('statistics.identity.named', {
                id: account.site_id,
                name: account.site_name,
              })}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('statistics.account.customerIdentity')}
            </dt>
            <dd className='break-words'>
              {t('statistics.identity.named', {
                id: account.customer_id,
                name: account.customer_name,
              })}
            </dd>
          </div>
          <div className='col-span-2'>
            <dt className='text-muted-foreground text-xs'>
              {t('statistics.account.remoteUserId')}
            </dt>
            <dd className='break-all'>{account.remote_user_id}</dd>
          </div>
        </dl>
      )}
      <section aria-label={t('statistics.siteBreakdown.title')}>
        <SiteBreakdownList sites={item.site_breakdown} />
      </section>
      <dl className='grid grid-cols-2 gap-3 text-sm'>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.request_count')}
          </dt>
          <dd>
            <MetricValue compact value={item.request_count} />
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.quota')}
          </dt>
          <dd>
            <MetricValue compact value={item.quota} />
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.token_used')}
          </dt>
          <dd>
            <MetricValue compact value={item.token_used} />
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.active_users')}
          </dt>
          <dd>
            <ActiveUsersValue compact scope={scope} value={item.active_users} />
          </dd>
        </div>
      </dl>
      <footer className='border-border grid gap-1 border-t pt-3'>
        <span className='text-sm'>
          {t('statistics.rowCompleteness', {
            rate: Math.round(item.completeness_rate * 1000) / 10,
          })}
          {' · '}
          {item.is_final
            ? t('statistics.final.final')
            : t('statistics.final.provisional')}
        </span>
        <DataFreshness labelKey='statistics.asOf' timestamp={item.as_of} />
      </footer>
    </article>
  )
}

function BreakdownTable<TBreakdown extends StatisticsBreakdownBase>({
  data,
  onSearchChange,
  scope,
  search,
}: {
  data: StatisticsResponse<TBreakdown>
  onSearchChange: (changes: Partial<StatisticsSearch>) => void
  scope: 'site' | 'customer' | 'account'
  search: StatisticsSearch
}) {
  const { t } = useTranslation()
  const emptyCopy = useStatisticsEmptyCopy(data.summary.data_status)
  const columns = useMemo<ColumnDef<TBreakdown, unknown>[]>(() => {
    const identityColumns: ColumnDef<TBreakdown, unknown>[] =
      scope !== 'account'
        ? [
            {
              cell: ({ row }) => (
                <SiteBreakdownList sites={row.original.site_breakdown} />
              ),
              header: t('statistics.siteBreakdown.title'),
              id: 'siteBreakdown',
            },
          ]
        : [
            {
              cell: ({ row }) => {
                const account =
                  row.original as unknown as AccountStatisticsBreakdown
                return (
                  <span className='whitespace-nowrap'>
                    {t('statistics.identity.named', {
                      id: account.site_id,
                      name: account.site_name,
                    })}
                  </span>
                )
              },
              header: t('statistics.account.siteIdentity'),
              id: 'siteIdentity',
            },
            {
              cell: ({ row }) => {
                const account =
                  row.original as unknown as AccountStatisticsBreakdown
                return (
                  <span className='whitespace-nowrap'>
                    {t('statistics.identity.named', {
                      id: account.customer_id,
                      name: account.customer_name,
                    })}
                  </span>
                )
              },
              header: t('statistics.account.customerIdentity'),
              id: 'customerIdentity',
            },
            {
              cell: ({ row }) => (
                <code>
                  {
                    (row.original as unknown as AccountStatisticsBreakdown)
                      .remote_user_id
                  }
                </code>
              ),
              header: t('statistics.account.remoteUserId'),
              id: 'remoteUserId',
            },
          ]
    return [
      {
        cell: ({ row }) => (
          <span className='whitespace-nowrap'>
            {formatBeijingTimestamp(
              row.original.bucket_start,
              search.granularity
            )}
          </span>
        ),
        enableSorting: true,
        header: t('statistics.bucket'),
        id: 'bucket_start',
      },
      {
        cell: ({ row }) => (
          <div className='min-w-32'>
            <span className='font-medium'>{row.original.dimension_name}</span>
            <code className='text-muted-foreground mt-1 block text-xs'>
              {t('statistics.identity.id', {
                id: row.original.dimension_id,
              })}
            </code>
          </div>
        ),
        header: t('statistics.dimension'),
        id: 'dimension',
      },
      ...identityColumns,
      {
        cell: ({ row }) => (
          <MetricValue compact value={row.original.request_count} />
        ),
        enableSorting: true,
        header: t('statistics.metric.request_count'),
        id: 'request_count',
      },
      {
        cell: ({ row }) => <MetricValue compact value={row.original.quota} />,
        enableSorting: true,
        header: t('statistics.metric.quota'),
        id: 'quota',
      },
      {
        cell: ({ row }) => (
          <AmountValue
            display={search.display}
            siteBreakdown={row.original.site_breakdown}
          />
        ),
        header: t(
          dynamicI18nKey('statistics', `statistics.display.${search.display}`)
        ),
        id: 'amount',
      },
      {
        cell: ({ row }) => (
          <MetricValue compact value={row.original.token_used} />
        ),
        enableSorting: true,
        header: t('statistics.metric.token_used'),
        id: 'token_used',
      },
      {
        cell: ({ row }) => (
          <ActiveUsersValue
            compact
            scope={scope}
            value={row.original.active_users}
          />
        ),
        enableSorting: true,
        header: t('statistics.metric.active_users'),
        id: 'active_users',
      },
      {
        cell: ({ row }) => (
          <div className='grid gap-1'>
            <DataStatusBadge status={row.original.data_status} />
            <span className='text-muted-foreground text-xs'>
              {t('statistics.rowCompleteness', {
                rate: Math.round(row.original.completeness_rate * 1000) / 10,
              })}
            </span>
            <span className='text-muted-foreground text-xs'>
              {row.original.is_final
                ? t('statistics.final.final')
                : t('statistics.final.provisional')}
            </span>
            <DataFreshness
              labelKey='statistics.asOf'
              timestamp={row.original.as_of}
            />
          </div>
        ),
        header: t('statistics.dataStatus'),
        id: 'dataStatus',
      },
    ]
  }, [scope, search.display, search.granularity, t])
  const updateSorting = (
    updater: SortingState | ((old: SortingState) => SortingState)
  ) => {
    const current = [{ desc: search.order === 'desc', id: search.sort }]
    const next = typeof updater === 'function' ? updater(current) : updater
    const first = next[0]
    if (!first || first.id === 'amount' || first.id === 'dataStatus') return
    onSearchChange({
      order: first.desc ? 'desc' : 'asc',
      page: 1,
      sort: first.id as StatisticsSearch['sort'],
    })
  }
  return (
    <DataTable
      ariaLabel={t('statistics.breakdownTable')}
      columns={columns}
      data={data.breakdown.items}
      emptyDescription={emptyCopy.description}
      emptyTitle={emptyCopy.title}
      onPageChange={(page) => onSearchChange({ page })}
      onSortingChange={updateSorting}
      page={search.page}
      pageSize={search.pageSize}
      renderMobileCard={(item) => (
        <BreakdownMobileCard
          granularity={search.granularity}
          item={item}
          scope={scope}
        />
      )}
      sorting={[{ desc: search.order === 'desc', id: search.sort }]}
      total={data.breakdown.total}
    />
  )
}

export function EntityStatistics<TBreakdown extends StatisticsBreakdownBase>({
  data,
  entityId,
  error,
  fetching,
  loading,
  onRetry,
  onSearchChange,
  rangeTransition,
  search,
  scope,
}: {
  data?: StatisticsResponse<TBreakdown>
  entityId: IdString
  error: boolean
  fetching: boolean
  loading: boolean
  onRetry: () => void
  onSearchChange: (changes: Partial<StatisticsSearch>) => void
  rangeTransition: boolean
  search: StatisticsSearch
  scope: 'site' | 'customer' | 'account'
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [exporting, setExporting] = useState(false)
  const [recreating, setRecreating] = useState(false)
  const [exportJob, setExportJob] = useState<StatisticsExportJobItem>()
  const [exportDraft, setExportDraft] = useState<{
    search: StatisticsSearch
    summaryStatus: StatisticsResponse['summary']['data_status']
    completeness: StatisticsResponse['completeness']
  } | null>(null)
  const exportStatistics = async (format: StatisticsExportFormat) => {
    if (!exportDraft) return
    setExporting(true)
    try {
      const job = await createStatisticsExport(
        buildEntityExportRequest(scope, entityId, format, exportDraft.search)
      )
      toast.success(
        job.deduplicated
          ? t('statistics.export.toast.deduplicated')
          : t('statistics.export.toast.created')
      )
      queryClient.setQueryData(statisticsKeys.export(job.id), job)
      void queryClient.invalidateQueries({
        queryKey: statisticsKeys.exportLists(),
      })
      setExportJob(job)
      setExportDraft(null)
      onSearchChange({ exportId: job.id })
    } catch (exportError) {
      toast.error(
        t(dynamicI18nKey('api', getApiErrorTranslationKey(exportError)))
      )
    } finally {
      setExporting(false)
    }
  }
  const recreateExport = async (job: StatisticsExportJobItem) => {
    setRecreating(true)
    try {
      const next = await createStatisticsExport({
        filters: job.filters,
        format: job.format,
        statistics_type: job.statistics_type,
      })
      toast.success(
        next.deduplicated
          ? t('statistics.export.toast.deduplicated')
          : t('statistics.export.toast.created')
      )
      queryClient.setQueryData(statisticsKeys.export(next.id), next)
      void queryClient.invalidateQueries({
        queryKey: statisticsKeys.exportLists(),
      })
      setExportJob(next)
      onSearchChange({ exportId: next.id })
    } catch (exportError) {
      toast.error(
        t(dynamicI18nKey('api', getApiErrorTranslationKey(exportError)))
      )
    } finally {
      setRecreating(false)
    }
  }
  let refreshStatus: ReactNode = null
  if (rangeTransition) {
    refreshStatus = (
      <p
        className='border-primary/25 bg-primary/5 flex items-center gap-2 rounded-md border p-3 text-sm'
        role='status'
      >
        <Spinner />
        {t('statistics.loadingNewRange')}
      </p>
    )
  } else if (fetching) {
    refreshStatus = (
      <p
        className='text-muted-foreground flex items-center gap-2 text-xs'
        role='status'
      >
        <Spinner />
        {t('table.refreshing')}
      </p>
    )
  }
  let body: ReactNode
  if (loading && !data) {
    body = (
      <div
        className='bg-muted h-64 animate-pulse rounded-lg'
        aria-hidden='true'
      />
    )
  } else if (error && !data) {
    body = (
      <section className='border-destructive/30 bg-destructive/5 rounded-lg border p-5'>
        <h2 className='font-medium'>{t('statistics.loadError')}</h2>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t('statistics.loadErrorDescription')}
        </p>
        <Button className='mt-3' onClick={onRetry} variant='outline'>
          {t('common.retry')}
        </Button>
      </section>
    )
  } else if (!data) {
    body = null
  } else {
    body = (
      <>
        {refreshStatus}
        {error && (
          <p
            className='border-warning/40 bg-warning/10 rounded-md border p-3 text-sm'
            role='status'
          >
            {t('statistics.staleData')}
          </p>
        )}
        <StatisticsSummary data={data} search={search} />
        <section
          aria-labelledby='statistics-trend-title'
          className='grid gap-3'
        >
          <h2 className='text-lg font-semibold' id='statistics-trend-title'>
            {t('statistics.trend')}
          </h2>
          {search.view === 'chart' ? (
            <MetricTrendChart data={data.trend} search={search} />
          ) : (
            <BreakdownTable
              data={data}
              onSearchChange={onSearchChange}
              scope={scope}
              search={search}
            />
          )}
        </section>
        <CompletenessAlert completeness={data.completeness} />
      </>
    )
  }
  return (
    <div className='grid min-w-0 gap-8'>
      <StatisticsToolbar
        exportDisabled={!data || rangeTransition}
        onExportOpen={() => {
          if (!data || rangeTransition) return
          setExportDraft({
            completeness: data.completeness,
            search: { ...search },
            summaryStatus: data.summary.data_status,
          })
        }}
        onSearchChange={onSearchChange}
        search={search}
      />
      {body}
      {exportDraft && (
        <ExportDialog
          completeness={exportDraft.completeness}
          entityId={entityId}
          onConfirm={(format) => void exportStatistics(format)}
          onOpenChange={(open) => !open && setExportDraft(null)}
          pending={exporting}
          scope={scope}
          search={exportDraft.search}
          summaryStatus={exportDraft.summaryStatus}
        />
      )}
      <ExportTaskSheet
        exportId={search.exportId}
        initialJob={exportJob}
        onOpenChange={(open) => {
          if (!open) onSearchChange({ exportId: undefined })
        }}
        onRecreate={(job) => void recreateExport(job)}
        recreating={recreating}
      />
    </div>
  )
}
