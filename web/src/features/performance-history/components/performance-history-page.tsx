import {
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
import { FilterPanel } from '@/components/data/filter-panel'
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
import {
  OperationsAnalyticsNavigation,
  OperationsViewPurpose,
} from '@/features/operations-analytics/components/operations-analytics-workspace'
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
import { getApiErrorTranslationKey, normalizeApiError } from '@/lib/api'
import { isIdString, parseIdString, parseMetricString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import { hasFilterChanges } from '@/lib/filter-state'

import {
  getPerformanceHistoryStatistics,
  getSitePerformanceHistoryStatistics,
  listPerformanceHistory,
  listSitePerformanceHistory,
} from '../api'
import { buildPerformanceHistoryExportRequest } from '../export-request'
import {
  formatPerformanceTps,
  millisecondsToSeconds,
  successRateToPercent,
  trustedWeightedSummary,
} from '../presentation'
import { performanceHistoryKeys } from '../query-keys'
import {
  buildPerformanceHistorySearch,
  type PerformanceHistorySearch,
} from '../search'
import type {
  PerformanceDimensionBreakdown,
  PerformanceHistoryItem,
  PerformanceHistoryQueryParams,
  PerformanceMetricSource,
  PerformanceWeightedMetric,
} from '../types'
import { PerformanceTimeRangePicker } from './performance-time-range-picker'
import { PerformanceTrendChart } from './performance-trend-chart'

function timestamp(value: number | null) {
  if (value == null || value <= 0) return '-'
  return fromUnixSeconds(value).format('YYYY-MM-DD HH:mm:ss')
}

function queryParams(search: PerformanceHistorySearch) {
  return {
    end_timestamp: search.end,
    groups: search.groups,
    model_names: search.models,
    p: search.page,
    page_size: search.pageSize,
    site_ids: search.siteIds,
    start_timestamp: search.start,
  } satisfies PerformanceHistoryQueryParams
}

function sourceLabel(
  source: PerformanceMetricSource,
  t: (key: string) => string
) {
  return source === 'counter_ready'
    ? t('performanceHistory.source.counterReady')
    : t('performanceHistory.source.officialAverage')
}

function MetricSourceBadge({ source }: { source: PerformanceMetricSource }) {
  const { t } = useTranslation()
  return (
    <Badge variant={source === 'counter_ready' ? 'success' : 'neutral'}>
      {sourceLabel(source, t)}
    </Badge>
  )
}

function SummaryGrid({ summary }: { summary: PerformanceWeightedMetric }) {
  const { t } = useTranslation()
  const items = [
    [t('performanceHistory.metric.requests'), summary.request_count, false],
    [t('performanceHistory.metric.successRate'), summary.success_rate, true],
    [
      t('performanceHistory.metric.latency'),
      millisecondsToSeconds(summary.avg_latency_ms),
      false,
    ],
    [
      t('performanceHistory.metric.ttft'),
      millisecondsToSeconds(summary.avg_ttft_ms),
      false,
    ],
    [
      t('performanceHistory.metric.tps'),
      formatPerformanceTps(summary.avg_tps),
      false,
    ],
  ] as const
  return (
    <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 xl:grid-cols-5'>
      {items.map(([label, value, percent]) => (
        <div className='border-border min-w-0 border-b p-3' key={label}>
          <dt className='text-muted-foreground text-xs'>{label}</dt>
          <dd className='mt-1 text-lg font-semibold break-all'>
            {percent ? (
              (successRateToPercent(value) ?? <MetricValue value={null} />)
            ) : (
              <MetricValue value={value} />
            )}
          </dd>
        </div>
      ))}
    </dl>
  )
}

function UnavailableSummary({ hasSamples }: { hasSamples: boolean }) {
  const { t } = useTranslation()
  const description = hasSamples
    ? t('performanceHistory.summaryUnavailable.description')
    : t('performanceHistory.summaryUnavailable.emptyDescription')
  return (
    <section className='border-border bg-muted/30 flex flex-wrap items-center gap-3 rounded-lg border px-4 py-3'>
      <div className='min-w-0 flex-1'>
        <p className='font-medium'>
          {t('performanceHistory.summaryUnavailable.title')}
        </p>
        <p className='text-muted-foreground text-sm'>{description}</p>
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
  onChange: (changes: Partial<PerformanceHistorySearch>) => void
  search: PerformanceHistorySearch
  sites: SiteListItem[]
}) {
  const { t } = useTranslation()
  const reset = buildPerformanceHistorySearch({
    hours: search.hours,
    pageSize: search.pageSize,
    view: search.view,
  })
  const stringList =
    (key: 'groups' | 'models') =>
    (event: React.ChangeEvent<HTMLInputElement>) =>
      onChange({
        [key]: event.target.value
          .split(',')
          .map((value) => value.trim())
          .filter(Boolean),
        page: 1,
      })
  const advancedCount = [
    search.models.length > 0,
    search.groups.length > 0,
  ].filter(Boolean).length

  return (
    <FilterPanel
      advanced={
        <>
          <label className='grid gap-1 text-sm'>
            <span>{t('performanceHistory.filters.models')}</span>
            <Input
              onChange={stringList('models')}
              value={search.models.join(',')}
            />
          </label>
          <label className='grid gap-1 text-sm'>
            <span>{t('performanceHistory.filters.groups')}</span>
            <Input
              onChange={stringList('groups')}
              value={search.groups.join(',')}
            />
          </label>
        </>
      }
      advancedCount={advancedCount}
      advancedMode='popover'
      description={t('performanceHistory.filters.description')}
      hasAdvancedActive={advancedCount > 0}
      hasActiveFilters={hasFilterChanges(search, reset, [
        'end',
        'groups',
        'models',
        'siteIds',
        'start',
      ])}
      onReset={() => onChange(reset)}
      title={t('performanceHistory.filters.title')}
    >
      <div className='flex min-w-0 flex-1 flex-wrap items-center gap-2'>
        <PerformanceTimeRangePicker onChange={onChange} search={search} />
        {global && (
          <FacetedFilter
            clearLabel={t('performanceHistory.filters.allSites')}
            onChange={(value) =>
              onChange({
                page: 1,
                siteIds: isIdString(value) ? [parseIdString(value)] : [],
              })
            }
            options={sites.map((site) => ({
              label: site.name,
              value: site.id,
            }))}
            title={t('performanceHistory.filters.site')}
            value={search.siteIds.length === 1 ? search.siteIds[0] : ''}
          />
        )}
      </div>
    </FilterPanel>
  )
}

function SiteRowsBreakdown({ items }: { items: PerformanceHistoryItem[] }) {
  const { t } = useTranslation()
  const sites = useMemo(() => {
    const grouped = new Map<
      string,
      { items: PerformanceHistoryItem[]; name: string }
    >()
    for (const item of items) {
      const current = grouped.get(item.site_id)
      if (current) current.items.push(item)
      else grouped.set(item.site_id, { items: [item], name: item.site_name })
    }
    return [...grouped.entries()]
  }, [items])

  if (sites.length === 0) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('performanceHistory.raw.empty')}
      </p>
    )
  }

  return (
    <section
      aria-label={t('performanceHistory.siteBreakdown.aria')}
      className='grid gap-5'
    >
      {sites.map(([siteId, site]) => (
        <RawRowsTable
          items={site.items}
          key={siteId}
          label={t('performanceHistory.siteBreakdown.siteAria', {
            id: siteId,
            name: site.name,
          })}
          title={t('performanceHistory.siteIdentity', {
            id: siteId,
            name: site.name,
          })}
        />
      ))}
    </section>
  )
}

function AggregateBreakdown({
  items,
  kind,
}: {
  items: PerformanceDimensionBreakdown[]
  kind: 'group' | 'model'
}) {
  const { t } = useTranslation()
  if (items.length === 0) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('performanceHistory.aggregate.empty')}
      </p>
    )
  }
  return (
    <section
      aria-label={
        kind === 'model'
          ? t('performanceHistory.modelBreakdown.aria')
          : t('performanceHistory.groupBreakdown.aria')
      }
      className='grid gap-3 md:grid-cols-2'
    >
      {items.map((item) => (
        <article
          className='border-border bg-card grid min-w-0 gap-3 rounded-xl border p-4'
          key={item.dimension}
        >
          <h2 className='font-medium break-all'>
            {item.dimension || t('data.unavailable')}
          </h2>
          <dl className='grid grid-cols-2 gap-3 text-sm'>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('performanceHistory.metric.requests')}
              </dt>
              <dd>{item.request_count ?? t('data.unavailableValue')}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('performanceHistory.metric.successRate')}
              </dt>
              <dd>
                {successRateToPercent(item.success_rate) ??
                  t('data.unavailableValue')}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('performanceHistory.metric.ttft')}
              </dt>
              <dd>
                {millisecondsToSeconds(item.avg_ttft_ms) ??
                  t('data.unavailableValue')}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('performanceHistory.metric.latency')}
              </dt>
              <dd>
                {millisecondsToSeconds(item.avg_latency_ms) ??
                  t('data.unavailableValue')}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('performanceHistory.metric.tps')}
              </dt>
              <dd>
                {formatPerformanceTps(item.avg_tps) ??
                  t('data.unavailableValue')}
              </dd>
            </div>
          </dl>
        </article>
      ))}
    </section>
  )
}

function RawRowsTable({
  items,
  label,
  title,
}: {
  items: PerformanceHistoryItem[]
  label: string
  title: string
}) {
  const { t } = useTranslation()
  return (
    <section aria-label={label} className='grid min-w-0 gap-3'>
      <h2 className='text-lg font-semibold'>{title}</h2>
      {items.length === 0 ? (
        <p className='text-muted-foreground text-sm'>
          {t('performanceHistory.raw.empty')}
        </p>
      ) : (
        <div
          className='border-border overflow-x-auto rounded-lg border'
          tabIndex={0}
        >
          <table className='w-full min-w-5xl text-sm'>
            <thead className='bg-[var(--table-header)] text-left'>
              <tr>
                <th className='px-3 py-2'>{t('performanceHistory.bucket')}</th>
                <th className='px-3 py-2'>{t('performanceHistory.site')}</th>
                <th className='px-3 py-2'>
                  {t('performanceHistory.modelGroup')}
                </th>
                <th className='px-3 py-2'>
                  {t('performanceHistory.metric.ttft')}
                </th>
                <th className='px-3 py-2'>
                  {t('performanceHistory.metric.latency')}
                </th>
                <th className='px-3 py-2'>
                  {t('performanceHistory.metric.successRate')}
                </th>
                <th className='px-3 py-2'>
                  {t('performanceHistory.metric.tps')}
                </th>
                <th className='px-3 py-2'>{t('performanceHistory.source')}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr
                  className='border-t transition-colors hover:bg-[var(--table-header-hover)]'
                  key={`${item.id}:${item.bucket_start}`}
                >
                  <td className='px-3 py-2 whitespace-nowrap'>
                    {timestamp(item.bucket_start)}
                  </td>
                  <td className='px-3 py-2'>
                    <span className='block'>{item.site_name}</span>
                    <code className='text-muted-foreground text-xs'>
                      {item.site_id}
                    </code>
                  </td>
                  <td className='px-3 py-2'>
                    <span className='block break-all'>{item.model_name}</span>
                    <code className='text-muted-foreground text-xs'>
                      {item.group || '-'}
                    </code>
                  </td>
                  <td className='px-3 py-2'>
                    {millisecondsToSeconds(item.avg_ttft_ms)}
                  </td>
                  <td className='px-3 py-2'>
                    {millisecondsToSeconds(item.avg_latency_ms)}
                  </td>
                  <td className='px-3 py-2'>
                    {successRateToPercent(item.success_rate)}
                  </td>
                  <td className='px-3 py-2'>
                    {formatPerformanceTps(item.avg_tps)}
                  </td>
                  <td className='px-3 py-2'>
                    <MetricSourceBadge source={item.metric_source} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}

export function PerformanceHistoryPage({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<PerformanceHistorySearch>) => void
  search: PerformanceHistorySearch
  siteId?: string
}) {
  const { t } = useTranslation()
  const [initialJob, setInitialJob] = useState<StatisticsExportJobItem>()
  const validSiteId = siteId == null || isIdString(siteId)
  const currentParams = useMemo(() => queryParams(search), [search])
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
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? listSitePerformanceHistory(parseIdString(siteId), currentParams)
        : listPerformanceHistory(currentParams),
    queryKey:
      siteId && isIdString(siteId)
        ? performanceHistoryKeys.siteList(siteId, currentParams)
        : performanceHistoryKeys.globalList(currentParams),
  })
  const statisticsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? getSitePerformanceHistoryStatistics(
            parseIdString(siteId),
            currentParams
          )
        : getPerformanceHistoryStatistics(currentParams),
    queryKey:
      siteId && isIdString(siteId)
        ? performanceHistoryKeys.siteStatistics(siteId, currentParams)
        : performanceHistoryKeys.globalStatistics(currentParams),
  })
  const exportMutation = useMutation({
    mutationFn: (format: StatisticsExportFormat) =>
      createStatisticsExport(
        buildPerformanceHistoryExportRequest(
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
  const retainedScope = siteId ? `site:${siteId}` : 'global'
  const list = useRetainedQueryData(
    listQuery.data,
    listQuery.isError,
    retainedScope
  )
  const statistics = useRetainedQueryData(
    statisticsQuery.data,
    statisticsQuery.isError,
    retainedScope
  )
  const completeness =
    search.view === 'list' ? list?.completeness : statistics?.completeness
  const summary = statistics ? trustedWeightedSummary(statistics) : undefined
  const statisticsError = statisticsQuery.isError
    ? normalizeApiError(statisticsQuery.error)
    : undefined
  const statisticsTooLarge = statisticsError?.code === 'PAYLOAD_TOO_LARGE'
  const purpose = {
    list: [
      t('performanceHistory.views.list'),
      t('performanceHistory.views.listDescription'),
    ],
    sites: [
      t('performanceHistory.views.sites'),
      t('performanceHistory.views.sitesDescription'),
    ],
    models: [
      t('performanceHistory.views.models'),
      t('performanceHistory.views.modelsDescription'),
    ],
    groups: [
      t('performanceHistory.views.groups'),
      t('performanceHistory.views.groupsDescription'),
    ],
    trend: [
      t('performanceHistory.views.trend'),
      t('performanceHistory.views.trendDescription'),
    ],
  }[search.view]
  const columns = useMemo<ColumnDef<PerformanceHistoryItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => <time>{timestamp(row.original.bucket_start)}</time>,
        header: t('performanceHistory.bucket'),
        id: 'bucket',
      },
      {
        cell: ({ row }) => (
          <div className='min-w-36'>
            <span className='block'>{row.original.site_name}</span>
            <code className='text-muted-foreground text-xs'>
              {row.original.site_id}
            </code>
          </div>
        ),
        header: t('performanceHistory.site'),
        id: 'site',
      },
      {
        cell: ({ row }) => (
          <div className='min-w-40'>
            <span className='block break-all'>{row.original.model_name}</span>
            <code className='text-muted-foreground text-xs'>
              {row.original.group || '-'}
            </code>
          </div>
        ),
        header: t('performanceHistory.modelGroup'),
        id: 'modelGroup',
      },
      {
        cell: ({ row }) => (
          <dl className='grid min-w-44 gap-1 text-xs'>
            <div>
              {t('performanceHistory.metricValue.ttft', {
                value: millisecondsToSeconds(row.original.avg_ttft_ms),
              })}
            </div>
            <div>
              {t('performanceHistory.metricValue.latency', {
                value: millisecondsToSeconds(row.original.avg_latency_ms),
              })}
            </div>
            <div>
              {t('performanceHistory.metricValue.success', {
                value: successRateToPercent(row.original.success_rate),
              })}
            </div>
            <div>
              {t('performanceHistory.metricValue.tps', {
                value: formatPerformanceTps(row.original.avg_tps),
              })}
            </div>
          </dl>
        ),
        header: t('performanceHistory.metrics'),
        id: 'metrics',
      },
      {
        cell: ({ row }) => (
          <MetricSourceBadge source={row.original.metric_source} />
        ),
        header: t('performanceHistory.source'),
        id: 'source',
      },
      {
        cell: ({ row }) => timestamp(row.original.collected_at),
        header: t('performanceHistory.collectedAt'),
        id: 'collectedAt',
      },
    ],
    [t]
  )

  return (
    <SectionPageLayout
      actions={
        search.view === 'list'
          ? (['xlsx', 'csv'] as const).map((format) => (
              <Button
                disabled={exportMutation.isPending || !validSiteId}
                key={format}
                onClick={() => exportMutation.mutate(format)}
                variant='outline'
              >
                <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
                {t('performanceHistory.export', {
                  format: format.toUpperCase(),
                })}
              </Button>
            ))
          : undefined
      }
      description={
        siteId
          ? t('performanceHistory.siteDescription', { id: siteId })
          : t('performanceHistory.description')
      }
      title={
        siteId
          ? t('performanceHistory.siteTitle')
          : t('performanceHistory.title')
      }
      fixedContent
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('performanceHistory.backToSite')}
          </DetailBackLink>
        )}
        {siteId && (
          <OperationsAnalyticsNavigation active='performance' siteId={siteId} />
        )}
        {statistics?.aggregation_status === 'complete' && summary && (
          <SummaryGrid summary={summary} />
        )}
        {statistics?.aggregation_status === 'unavailable' && (
          <UnavailableSummary
            hasSamples={statistics.site_breakdown.length > 0}
          />
        )}
        <Tabs
          onValueChange={(view) =>
            onSearchChange({
              page: 1,
              view: view as PerformanceHistorySearch['view'],
            })
          }
          value={search.view}
        >
          <TabsList
            aria-label={t('performanceHistory.views.label')}
            className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'
          >
            <TabsTrigger value='list'>
              {t('performanceHistory.views.list')}
            </TabsTrigger>
            <TabsTrigger value='trend'>
              {t('performanceHistory.views.trend')}
            </TabsTrigger>
            <TabsTrigger value='models'>
              {t('performanceHistory.views.models')}
            </TabsTrigger>
            <TabsTrigger value='groups'>
              {t('performanceHistory.views.groups')}
            </TabsTrigger>
            <TabsTrigger value='sites'>
              {t('performanceHistory.views.sites')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <OperationsViewPurpose
          badges={
            <>
              {list && <DataStatusBadge status={list.data_status} />}
              {statistics && (
                <Badge
                  variant={
                    statistics.aggregation_status === 'complete'
                      ? 'success'
                      : 'warning'
                  }
                >
                  {statistics.aggregation_status === 'complete'
                    ? t('performanceHistory.aggregation.weighted')
                    : t('performanceHistory.aggregation.unavailable')}
                </Badge>
              )}
              {completeness && (
                <Badge variant='outline'>
                  {t('performanceHistory.completeness', {
                    complete: completeness.successful_site_count,
                    expected: completeness.expected_site_count,
                    unavailable: completeness.unavailable_site_count,
                  })}
                </Badge>
              )}
            </>
          }
          description={purpose[1]}
          icon={search.view === 'list' ? Database01Icon : Chart01Icon}
          notice={
            <>
              {t('performanceHistory.boundary.description')}
              {list && (
                <>
                  {' · '}
                  {t('performanceHistory.asOf', {
                    time: timestamp(list.as_of),
                  })}
                </>
              )}
            </>
          }
          title={purpose[0]}
        />
        <Filters
          global={!siteId}
          onChange={onSearchChange}
          search={search}
          sites={sitesQuery.data?.items ?? []}
        />
        {!siteId && sitesQuery.isError && (
          <QueryStateAlert
            message={t('operationsAnalytics.siteOptionsError')}
            onRetry={() => void sitesQuery.refetch()}
          />
        )}
        {search.view === 'list' && listQuery.isError && list && (
          <QueryStateAlert
            message={t('operationsAnalytics.staleListData')}
            onRetry={() => void listQuery.refetch()}
          />
        )}
        {statisticsQuery.isError && statistics && (
          <QueryStateAlert
            message={
              statisticsTooLarge
                ? t('performanceHistory.statisticsTooLarge')
                : t('operationsAnalytics.staleStatisticsData')
            }
            onRetry={() => void statisticsQuery.refetch()}
          />
        )}
        {search.view === 'list' && (
          <DataTable
            ariaLabel={t('performanceHistory.table')}
            columns={columns}
            data={list?.items ?? []}
            emptyDescription={
              list?.data_status === 'complete'
                ? t('performanceHistory.emptyCompleteDescription')
                : t('performanceHistory.emptyDescription')
            }
            emptyTitle={t('performanceHistory.empty')}
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
                  <div>
                    <p className='font-medium break-all'>{item.model_name}</p>
                    <code className='text-muted-foreground text-xs'>
                      {item.group || '-'}
                    </code>
                  </div>
                  <MetricSourceBadge source={item.metric_source} />
                </div>
                <p className='text-muted-foreground text-xs'>
                  {t('performanceHistory.siteIdentity', {
                    id: item.site_id,
                    name: item.site_name,
                  })}
                </p>
                <time className='text-sm'>{timestamp(item.bucket_start)}</time>
                <dl className='grid grid-cols-2 gap-3 text-sm'>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('performanceHistory.metric.ttft')}
                    </dt>
                    <dd>{millisecondsToSeconds(item.avg_ttft_ms)}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('performanceHistory.metric.latency')}
                    </dt>
                    <dd>{millisecondsToSeconds(item.avg_latency_ms)}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('performanceHistory.metric.successRate')}
                    </dt>
                    <dd>{successRateToPercent(item.success_rate)}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('performanceHistory.metric.tps')}
                    </dt>
                    <dd>{formatPerformanceTps(item.avg_tps)}</dd>
                  </div>
                </dl>
              </article>
            )}
            paginationHasKnownLastPage={false}
            paginationHasNextPage={
              list
                ? BigInt(list.total) >
                  BigInt(search.page) * BigInt(search.pageSize)
                : false
            }
            paginationTotalDisplay={
              <MetricValue value={list?.total ?? parseMetricString('0')} />
            }
          />
        )}
        {statisticsQuery.isError && !statistics && (
          <ErrorState
            className='min-h-40'
            description={
              statisticsTooLarge
                ? t('performanceHistory.statisticsTooLargeDescription')
                : undefined
            }
            onRetry={() => void statisticsQuery.refetch()}
            title={
              statisticsTooLarge
                ? t('performanceHistory.statisticsTooLarge')
                : t('performanceHistory.statisticsError')
            }
          />
        )}
        {statistics && search.view !== 'list' && (
          <div className='min-h-0 flex-1 overflow-y-auto pr-1' tabIndex={0}>
            {search.view === 'trend' && (
              <PerformanceTrendChart
                ariaLabel={t('performanceHistory.trend.chartAria')}
                emptyText={t('performanceHistory.raw.empty')}
                items={statistics.trend}
                latencyLabel={t('performanceHistory.metric.latency')}
                ttftLabel={t('performanceHistory.metric.ttft')}
              />
            )}
            {search.view === 'sites' && (
              <SiteRowsBreakdown items={statistics.site_breakdown} />
            )}
            {search.view === 'models' && (
              <AggregateBreakdown
                items={statistics.model_breakdown}
                kind='model'
              />
            )}
            {search.view === 'groups' && (
              <AggregateBreakdown
                items={statistics.group_breakdown}
                kind='group'
              />
            )}
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
