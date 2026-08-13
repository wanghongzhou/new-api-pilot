import {
  ArrowLeft01Icon,
  Chart01Icon,
  Database01Icon,
  FileExportIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
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
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  OperationsAnalyticsNavigation,
  OperationsViewPurpose,
} from '@/features/operations-analytics/components/operations-analytics-workspace'
import { listSites } from '@/features/sites/api'
import { siteKeys } from '@/features/sites/query-keys'
import { createStatisticsExport } from '@/features/statistics/api'
import { ExportTaskSheet } from '@/features/statistics/components/export-task-sheet'
import type {
  StatisticsExportFormat,
  StatisticsExportJobItem,
} from '@/features/statistics/types'
import { useRetainedQueryData } from '@/hooks/use-retained-query-data'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import {
  isIdString,
  isNonNegativeIdString,
  parseIdString,
} from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import { hasFilterChanges } from '@/lib/filter-state'

import { getRankings, getSiteRankings } from '../api'
import { buildRankingExportRequest } from '../export-request'
import { ratioToPercent } from '../presentation'
import { rankingKeys } from '../query-keys'
import { buildRankingSearch, type RankingSearch } from '../search'
import type {
  RankingHistoryPoint,
  RankingItem,
  RankingPeriod,
  RankingSiteBreakdown,
} from '../types'

function time(value: number | null) {
  return value == null || value <= 0
    ? '-'
    : fromUnixSeconds(value).format('YYYY-MM-DD HH:mm:ss')
}

function pageItems<T>(items: T[], page: number, pageSize: number) {
  const start = (page - 1) * pageSize
  return items.slice(start, start + pageSize)
}

function periodText(period: RankingPeriod, t: (key: string) => string) {
  switch (period) {
    case 'week':
      return t('rankings.period.week')
    case 'month':
      return t('rankings.period.month')
    case 'year':
      return t('rankings.period.year')
    default:
      return t('rankings.period.today')
  }
}

function name(item: RankingItem, vendors: boolean, t: (key: string) => string) {
  return vendors &&
    isNonNegativeIdString(item.dimension_id) &&
    item.dimension_id === '0'
    ? t('rankings.unknownVendor')
    : item.dimension_name || item.dimension_id
}

function movementText(item: RankingItem, t: (key: string) => string) {
  if (item.movement_type === 'new') return t('rankings.movement.new')
  if (item.movement_type === 'removed') return t('rankings.movement.removed')
  return ratioToPercent(item.growth) ?? t('data.unavailable')
}

function RankingMetric({
  label,
  value,
}: {
  label: string
  value: RankingItem['token_used']
}) {
  return (
    <span>
      {label}：<MetricValue value={value} />
    </span>
  )
}

function RankingList({
  items,
  title,
  vendors,
}: {
  items: RankingItem[]
  title: string
  vendors: boolean
}) {
  const { t } = useTranslation()
  return (
    <section className='grid gap-2'>
      <h3 className='font-semibold'>{title}</h3>
      {items.length === 0 && (
        <p className='text-muted-foreground rounded-lg border border-dashed p-4 text-sm'>
          {t('rankings.emptyMovement')}
        </p>
      )}
      {items.slice(0, 10).map((item) => (
        <article
          className='border-border grid gap-1 rounded-lg border p-3'
          key={item.dimension_id}
        >
          <div className='flex justify-between gap-2'>
            <span className='font-medium'>
              {item.movement_type === 'removed' ? '' : `#${item.rank} `}
              {name(item, vendors, t)}
            </span>
            <span>{movementText(item, t)}</span>
          </div>
          <span className='text-muted-foreground text-xs'>
            <RankingMetric
              label={t('rankings.tokens')}
              value={item.token_used}
            />
            ；{t('rankings.shareValue', { value: ratioToPercent(item.share) })}
          </span>
        </article>
      ))}
    </section>
  )
}

export function RankingsPage({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<RankingSearch>) => void
  search: RankingSearch
  siteId?: string
}) {
  const { t } = useTranslation()
  const [initialJob, setInitialJob] = useState<StatisticsExportJobItem>()
  const validSite = siteId == null || isIdString(siteId)
  const queryParams = useMemo(
    () => ({ period: search.period, site_ids: search.siteIds }),
    [search.period, search.siteIds]
  )
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
  const rankingQuery = useQuery({
    enabled: validSite,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? getSiteRankings(parseIdString(siteId), search.tab, queryParams)
        : getRankings(search.tab, queryParams),
    queryKey:
      siteId && isIdString(siteId)
        ? rankingKeys.site(siteId, search.tab, queryParams)
        : rankingKeys.global(search.tab, queryParams),
  })
  const exportMutation = useMutation({
    mutationFn: (format: StatisticsExportFormat) =>
      createStatisticsExport(
        buildRankingExportRequest(
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
  const data = useRetainedQueryData(
    rankingQuery.data,
    rankingQuery.isError,
    `${siteId ? `site:${siteId}` : 'global'}:${search.tab}`
  )
  const vendors = search.tab === 'vendors'
  const columns = useMemo<ColumnDef<RankingItem, unknown>[]>(
    () => [
      { accessorKey: 'rank', header: t('rankings.rank') },
      {
        cell: ({ row }) => (
          <div>
            <span className='font-medium'>
              {name(row.original, vendors, t)}
            </span>
            <code className='text-muted-foreground block text-xs'>
              {row.original.dimension_id}
            </code>
          </div>
        ),
        header: t('rankings.dimension'),
        id: 'dimension',
      },
      {
        cell: ({ row }) => (
          <div className='grid gap-1 text-xs'>
            <RankingMetric
              label={t('rankings.tokens')}
              value={row.original.token_used}
            />
            <RankingMetric
              label={t('rankings.requests')}
              value={row.original.request_count}
            />
            <RankingMetric
              label={t('rankings.quota')}
              value={row.original.quota}
            />
          </div>
        ),
        header: t('rankings.totals'),
        id: 'totals',
      },
      {
        cell: ({ row }) => ratioToPercent(row.original.share),
        header: t('rankings.share'),
        id: 'share',
      },
      {
        cell: ({ row }) =>
          ratioToPercent(row.original.growth) ?? t('data.unavailable'),
        header: t('rankings.growth'),
        id: 'growth',
      },
    ],
    [t, vendors]
  )
  const historyColumns = useMemo<ColumnDef<RankingHistoryPoint, unknown>[]>(
    () => [
      {
        cell: ({ row }) => <time>{time(row.original.bucket_start)}</time>,
        header: t('rankings.bucket'),
        id: 'bucket',
      },
      {
        accessorKey: 'dimension_id',
        header: t('rankings.dimension'),
      },
      {
        accessorKey: 'token_used',
        header: t('rankings.tokens'),
      },
    ],
    [t]
  )
  const siteColumns = useMemo<ColumnDef<RankingSiteBreakdown, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='min-w-36'>
            <span className='block'>{row.original.site_name}</span>
            <code className='text-muted-foreground text-xs'>
              {row.original.site_id}
            </code>
          </div>
        ),
        header: t('rankings.filters.site'),
        id: 'site',
      },
      {
        accessorKey: 'dimension_id',
        header: t('rankings.dimension'),
      },
      {
        accessorKey: 'token_used',
        header: t('rankings.tokens'),
      },
      {
        cell: ({ row }) => (
          <DataStatusBadge status={row.original.data_status} />
        ),
        header: t('rankings.dataStatus'),
        id: 'dataStatus',
      },
      {
        cell: ({ row }) => time(row.original.as_of),
        header: t('rankings.asOfLabel'),
        id: 'asOf',
      },
    ],
    [t]
  )
  const periods: RankingPeriod[] = ['today', 'week', 'month', 'year']
  const reset = buildRankingSearch({
    pageSize: search.pageSize,
    tab: search.tab,
    view: search.view,
  })
  const purpose = {
    history: [
      t('rankings.views.history'),
      t('rankings.views.historyDescription'),
    ],
    movement: [
      t('rankings.views.movement'),
      t('rankings.views.movementDescription'),
    ],
    ranking: [
      t('rankings.views.ranking'),
      t('rankings.views.rankingDescription'),
    ],
    sites: [t('rankings.views.sites'), t('rankings.views.sitesDescription')],
  }[search.view]
  return (
    <SectionPageLayout
      actions={
        search.view === 'ranking'
          ? (['xlsx', 'csv'] as const).map((format) => (
              <Button
                disabled={exportMutation.isPending || !validSite}
                key={format}
                onClick={() => exportMutation.mutate(format)}
                variant='outline'
              >
                <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
                {t('rankings.export', { format: format.toUpperCase() })}
              </Button>
            ))
          : undefined
      }
      description={
        siteId
          ? t('rankings.siteDescription', { id: siteId })
          : t('rankings.description')
      }
      fixedContent
      title={siteId ? t('rankings.siteTitle') : t('rankings.title')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('rankings.backToSite')}
          </DetailBackLink>
        )}
        {siteId && (
          <OperationsAnalyticsNavigation active='rankings' siteId={siteId} />
        )}
        <Tabs
          onValueChange={(tab) =>
            onSearchChange({ page: 1, tab: tab as RankingSearch['tab'] })
          }
          value={search.tab}
        >
          <TabsList aria-label={t('rankings.tabs.label')}>
            <TabsTrigger value='models'>
              {t('rankings.tabs.models')}
            </TabsTrigger>
            <TabsTrigger value='vendors'>
              {t('rankings.tabs.vendors')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <Tabs
          onValueChange={(view) =>
            onSearchChange({
              page: 1,
              view: view as RankingSearch['view'],
            })
          }
          value={search.view}
        >
          <TabsList
            aria-label={t('rankings.views.label')}
            className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'
          >
            <TabsTrigger value='ranking'>
              {t('rankings.views.ranking')}
            </TabsTrigger>
            <TabsTrigger value='movement'>
              {t('rankings.views.movement')}
            </TabsTrigger>
            <TabsTrigger value='history'>
              {t('rankings.views.history')}
            </TabsTrigger>
            <TabsTrigger value='sites'>{t('rankings.views.sites')}</TabsTrigger>
          </TabsList>
        </Tabs>
        <OperationsViewPurpose
          badges={data && <DataStatusBadge status={data.data_status} />}
          description={purpose[1]}
          icon={search.view === 'ranking' ? Database01Icon : Chart01Icon}
          notice={
            data ? (
              <>
                {t('rankings.range', {
                  end: time(data.end_timestamp),
                  start: time(data.start_timestamp),
                })}
                {' · '}
                {t('rankings.asOf', { time: time(data.as_of) })}
                {' · '}
                {t('rankings.localBoundary')}
              </>
            ) : (
              t('rankings.localBoundary')
            )
          }
          title={purpose[0]}
        />
        <FilterPanel
          description={t('rankings.localBoundary')}
          hasActiveFilters={hasFilterChanges(search, reset, [
            'period',
            'siteIds',
          ])}
          onReset={() => onSearchChange(reset)}
          title={t('rankings.tabs.label')}
        >
          <div className='flex flex-wrap gap-2'>
            {periods.map((period) => (
              <Button
                aria-pressed={search.period === period}
                key={period}
                onClick={() => onSearchChange({ page: 1, period })}
                variant={search.period === period ? 'secondary' : 'outline'}
              >
                {periodText(period, t)}
              </Button>
            ))}
          </div>
          {!siteId && (
            <FacetedFilter
              clearLabel={t('rankings.filters.allSites')}
              onChange={(value) =>
                onSearchChange({
                  page: 1,
                  siteIds: isIdString(value) ? [parseIdString(value)] : [],
                })
              }
              options={(sitesQuery.data?.items ?? []).map((site) => ({
                label: site.name,
                value: site.id,
              }))}
              title={t('rankings.filters.site')}
              value={search.siteIds.length === 1 ? search.siteIds[0] : ''}
            />
          )}
        </FilterPanel>
        {!siteId && sitesQuery.isError && (
          <QueryStateAlert
            message={t('operationsAnalytics.siteOptionsError')}
            onRetry={() => void sitesQuery.refetch()}
          />
        )}
        {rankingQuery.isError && data && (
          <QueryStateAlert
            message={t('operationsAnalytics.staleListData')}
            onRetry={() => void rankingQuery.refetch()}
          />
        )}
        {search.view === 'ranking' && (
          <DataTable
            ariaLabel={t('rankings.table')}
            columns={columns}
            data={pageItems(data?.items ?? [], search.page, search.pageSize)}
            emptyTitle={t('rankings.empty')}
            error={!validSite || rankingQuery.isError}
            loading={rankingQuery.isPending}
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={validSite ? () => void rankingQuery.refetch() : undefined}
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(item) => (
              <article className='border-border grid gap-2 rounded-lg border p-4'>
                <div className='flex justify-between gap-2'>
                  <span className='font-medium'>
                    #{item.rank} {name(item, vendors, t)}
                  </span>
                  <span>
                    {ratioToPercent(item.growth) ?? t('data.unavailable')}
                  </span>
                </div>
                <RankingMetric
                  label={t('rankings.tokens')}
                  value={item.token_used}
                />
                <RankingMetric
                  label={t('rankings.requests')}
                  value={item.request_count}
                />
                <RankingMetric label={t('rankings.quota')} value={item.quota} />
                <span>
                  {t('rankings.shareValue', {
                    value: ratioToPercent(item.share),
                  })}
                </span>
              </article>
            )}
            total={data?.items.length ?? 0}
          />
        )}
        {search.view !== 'ranking' && rankingQuery.isPending && (
          <LoadingState message={t('common.loading')} />
        )}
        {search.view !== 'ranking' && rankingQuery.isError && !data && (
          <ErrorState
            onRetry={validSite ? () => void rankingQuery.refetch() : undefined}
            title={t('rankings.loadError')}
          />
        )}
        {data && search.view === 'movement' && (
          <div className='min-h-0 flex-1 overflow-y-auto pr-1' tabIndex={0}>
            <div className='grid gap-6 xl:grid-cols-2'>
              <RankingList
                items={data.movers}
                title={t('rankings.movers')}
                vendors={vendors}
              />
              <RankingList
                items={data.droppers}
                title={t('rankings.droppers')}
                vendors={vendors}
              />
            </div>
          </div>
        )}
        {data && search.view === 'history' && (
          <DataTable
            ariaLabel={t('rankings.history')}
            columns={historyColumns}
            data={pageItems(data.history, search.page, search.pageSize)}
            emptyTitle={t('rankings.emptyHistory')}
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(point) => (
              <article className='border-border grid gap-2 rounded-lg border p-4'>
                <time>{time(point.bucket_start)}</time>
                <code className='text-muted-foreground text-xs break-all'>
                  {point.dimension_id}
                </code>
                <RankingMetric
                  label={t('rankings.tokens')}
                  value={point.token_used}
                />
              </article>
            )}
            total={data.history.length}
          />
        )}
        {data && search.view === 'sites' && (
          <DataTable
            ariaLabel={t('rankings.siteBreakdown')}
            columns={siteColumns}
            data={pageItems(data.site_breakdown, search.page, search.pageSize)}
            emptyTitle={t('rankings.emptySites')}
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(item) => (
              <article className='border-border grid gap-2 rounded-lg border p-4'>
                <span className='font-medium'>{item.site_name}</span>
                <code className='text-muted-foreground text-xs'>
                  {item.site_id}
                </code>
                <span>{item.dimension_id}</span>
                <RankingMetric
                  label={t('rankings.tokens')}
                  value={item.token_used}
                />
                <div className='flex flex-wrap items-center justify-between gap-2'>
                  <DataStatusBadge status={item.data_status} />
                  <span className='text-muted-foreground text-xs'>
                    {time(item.as_of)}
                  </span>
                </div>
              </article>
            )}
            total={data.site_breakdown.length}
          />
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
