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
import { isIdString, parseIdString } from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'
import { hasFilterChanges } from '@/lib/filter-state'

import {
  getPerformanceHistoryStatistics,
  getSitePerformanceHistoryStatistics,
  listPerformanceHistory,
  listSitePerformanceHistory,
} from '../api'
import { buildPerformanceHistoryExportRequest } from '../export-request'
import { trustedWeightedSummary } from '../presentation'
import { performanceHistoryKeys } from '../query-keys'
import {
  buildPerformanceHistorySearch,
  performanceRangeForHours,
  type PerformanceHistorySearch,
} from '../search'
import type {
  PerformanceHistoryItem,
  PerformanceHistoryQueryParams,
  PerformanceMetricSource,
  PerformanceWeightedMetric,
} from '../types'

function timestamp(value: number | null) {
  if (value == null || value <= 0) return '-'
  return fromUnixSeconds(value).format('YYYY-MM-DD HH:mm:ss')
}

function dateTimeValue(value: number) {
  return fromUnixSeconds(value).format('YYYY-MM-DDTHH:mm')
}

function parseDateTime(value: string) {
  const parsed = dayjs.tz(value, 'YYYY-MM-DDTHH:mm', BEIJING_TIMEZONE)
  return parsed.isValid() ? parsed.startOf('hour').unix() : undefined
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

function hoursLabel(hours: 24 | 168 | 720, t: (key: string) => string) {
  if (hours === 24) return t('performanceHistory.filters.hours24')
  if (hours === 168) return t('performanceHistory.filters.hours168')
  return t('performanceHistory.filters.hours720')
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
    [t('performanceHistory.metric.requests'), summary.request_count],
    [t('performanceHistory.metric.successRate'), summary.success_rate],
    [t('performanceHistory.metric.latency'), summary.avg_latency_ms],
    [t('performanceHistory.metric.ttft'), summary.avg_ttft_ms],
    [t('performanceHistory.metric.tps'), summary.avg_tps],
  ] as const
  return (
    <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 xl:grid-cols-5'>
      {items.map(([label, value]) => (
        <div className='border-border min-w-0 border-b p-3' key={label}>
          <dt className='text-muted-foreground text-xs'>{label}</dt>
          <dd className='mt-1 text-lg font-semibold break-all'>
            <MetricValue value={value} />
          </dd>
        </div>
      ))}
    </dl>
  )
}

function Filters({
  global,
  onChange,
  search,
}: {
  global: boolean
  onChange: (changes: Partial<PerformanceHistorySearch>) => void
  search: PerformanceHistorySearch
}) {
  const { t } = useTranslation()
  const reset = buildPerformanceHistorySearch({
    hours: search.hours,
    pageSize: search.pageSize,
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

  return (
    <FilterPanel
      description={t('performanceHistory.filters.description')}
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
      <fieldset className='grid gap-2'>
        <legend className='text-sm'>
          {t('performanceHistory.filters.hours')}
        </legend>
        <div className='flex flex-wrap gap-2'>
          {([24, 168, 720] as const).map((hours) => (
            <Button
              aria-pressed={search.hours === hours}
              key={hours}
              onClick={() => onChange(performanceRangeForHours(hours))}
              size='sm'
              type='button'
              variant={search.hours === hours ? 'secondary' : 'outline'}
            >
              {hoursLabel(hours, t)}
            </Button>
          ))}
        </div>
      </fieldset>
      <div className='grid min-w-0 flex-1 gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        {global && (
          <label className='grid gap-1 text-sm'>
            <span>{t('performanceHistory.filters.siteIds')}</span>
            <Input
              onChange={(event) =>
                onChange({
                  page: 1,
                  siteIds: event.target.value
                    .split(',')
                    .map((value) => value.trim())
                    .filter(isIdString)
                    .map(parseIdString),
                })
              }
              placeholder={t('performanceHistory.filters.siteIdsPlaceholder')}
              value={search.siteIds.join(',')}
            />
          </label>
        )}
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
        <label className='grid gap-1 text-sm'>
          <span>{t('performanceHistory.filters.start')}</span>
          <Input
            onChange={(event) => {
              const start = parseDateTime(event.target.value)
              if (start != null) {
                onChange({ hours: search.hours, page: 1, start })
              }
            }}
            type='datetime-local'
            value={dateTimeValue(search.start)}
          />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('performanceHistory.filters.end')}</span>
          <Input
            onChange={(event) => {
              const end = parseDateTime(event.target.value)
              if (end != null) onChange({ end, hours: search.hours, page: 1 })
            }}
            type='datetime-local'
            value={dateTimeValue(search.end)}
          />
        </label>
      </div>
    </FilterPanel>
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
        <div className='border-border overflow-x-auto rounded-lg border'>
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
                  <td className='px-3 py-2'>{item.avg_ttft_ms}</td>
                  <td className='px-3 py-2'>{item.avg_latency_ms}</td>
                  <td className='px-3 py-2'>{item.success_rate}</td>
                  <td className='px-3 py-2'>{item.avg_tps}</td>
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
  const list = listQuery.data
  const statistics = statisticsQuery.data
  const summary = statistics ? trustedWeightedSummary(statistics) : undefined
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
                value: row.original.avg_ttft_ms,
              })}
            </div>
            <div>
              {t('performanceHistory.metricValue.latency', {
                value: row.original.avg_latency_ms,
              })}
            </div>
            <div>
              {t('performanceHistory.metricValue.success', {
                value: row.original.success_rate,
              })}
            </div>
            <div>
              {t('performanceHistory.metricValue.tps', {
                value: row.original.avg_tps,
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
      actions={(['xlsx', 'csv'] as const).map((format) => (
        <Button
          disabled={exportMutation.isPending || !validSiteId}
          key={format}
          onClick={() => exportMutation.mutate(format)}
          variant='outline'
        >
          <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
          {t('performanceHistory.export', { format: format.toUpperCase() })}
        </Button>
      ))}
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
    >
      <div className='grid min-w-0 gap-6'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('performanceHistory.backToSite')}
          </DetailBackLink>
        )}
        <section
          className='border-border bg-muted/30 rounded-lg border p-4'
          role='note'
        >
          <p className='font-medium'>
            {t('performanceHistory.boundary.title')}
          </p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('performanceHistory.boundary.description')}
          </p>
        </section>
        <Filters global={!siteId} onChange={onSearchChange} search={search} />
        <div className='grid gap-3 sm:grid-cols-2'>
          {list && (
            <section
              className='border-border flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3'
              role='status'
            >
              <div className='flex items-center gap-2'>
                <span className='text-sm font-medium'>
                  {t('performanceHistory.dataStatus')}
                </span>
                <DataStatusBadge status={list.data_status} />
              </div>
              <span className='text-muted-foreground text-xs'>
                {t('performanceHistory.asOf', { time: timestamp(list.as_of) })}
              </span>
            </section>
          )}
          {statistics && (
            <section
              className='border-border flex items-center gap-2 rounded-lg border p-3'
              role='status'
            >
              <span className='text-sm font-medium'>
                {t('performanceHistory.aggregation.title')}
              </span>
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
            </section>
          )}
        </div>
        {statistics?.aggregation_status === 'unavailable' && (
          <section
            className='border-warning/30 bg-warning/5 rounded-lg border p-4'
            role='alert'
          >
            <p className='font-medium'>
              {t('performanceHistory.unavailable.title')}
            </p>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('performanceHistory.unavailable.description')}
            </p>
          </section>
        )}
        {summary && <SummaryGrid summary={summary} />}
        <DataTable
          ariaLabel={t('performanceHistory.table')}
          columns={columns}
          data={list?.items ?? []}
          emptyDescription={t('performanceHistory.emptyDescription')}
          emptyTitle={t('performanceHistory.empty')}
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
                  <dd>{item.avg_ttft_ms}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('performanceHistory.metric.latency')}
                  </dt>
                  <dd>{item.avg_latency_ms}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('performanceHistory.metric.successRate')}
                  </dt>
                  <dd>{item.success_rate}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('performanceHistory.metric.tps')}
                  </dt>
                  <dd>{item.avg_tps}</dd>
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
            title={t('performanceHistory.statisticsError')}
          />
        )}
        {statistics && (
          <>
            <RawRowsTable
              items={statistics.trend}
              label={t('performanceHistory.trend.aria')}
              title={t('performanceHistory.trend.title')}
            />
            <RawRowsTable
              items={statistics.site_breakdown}
              label={t('performanceHistory.siteBreakdown.aria')}
              title={t('performanceHistory.siteBreakdown.title')}
            />
          </>
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
