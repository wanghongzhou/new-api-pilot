import { ArrowLeft01Icon, FileExportIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
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
} from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'

import { getRankings, getSiteRankings } from '../api'
import { buildRankingExportRequest } from '../export-request'
import { rankingKeys } from '../query-keys'
import type { RankingSearch } from '../search'
import type { RankingItem, RankingPeriod } from '../types'

function time(value: number | null) {
  return value == null || value <= 0
    ? '-'
    : fromUnixSeconds(value).format('YYYY-MM-DD HH:mm:ss')
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
      {items.slice(0, 10).map((item) => (
        <article
          className='border-border grid gap-1 rounded-lg border p-3'
          key={item.dimension_id}
        >
          <div className='flex justify-between gap-2'>
            <span className='font-medium'>
              #{item.rank} {name(item, vendors, t)}
            </span>
            <span>
              {item.growth == null ? t('data.unavailableValue') : item.growth}
            </span>
          </div>
          <span className='text-muted-foreground text-xs'>
            {t('rankings.itemValues', {
              share: item.share,
              tokens: item.token_used,
            })}
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
  const data = rankingQuery.data
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
            <span>
              {t('rankings.tokensValue', { value: row.original.token_used })}
            </span>
            <span>
              {t('rankings.requestsValue', {
                value: row.original.request_count,
              })}
            </span>
            <span>
              {t('rankings.quotaValue', { value: row.original.quota })}
            </span>
          </div>
        ),
        header: t('rankings.totals'),
        id: 'totals',
      },
      {
        cell: ({ row }) => row.original.share,
        header: t('rankings.share'),
        id: 'share',
      },
      {
        cell: ({ row }) => <MetricValue value={row.original.growth} />,
        header: t('rankings.growth'),
        id: 'growth',
      },
    ],
    [t, vendors]
  )
  const periods: RankingPeriod[] = ['today', 'week', 'month', 'year']
  return (
    <SectionPageLayout
      actions={(['xlsx', 'csv'] as const).map((format) => (
        <Button
          key={format}
          onClick={() => exportMutation.mutate(format)}
          variant='outline'
        >
          <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
          {t('rankings.export', { format: format.toUpperCase() })}
        </Button>
      ))}
      description={
        siteId
          ? t('rankings.siteDescription', { id: siteId })
          : t('rankings.description')
      }
      title={siteId ? t('rankings.siteTitle') : t('rankings.title')}
    >
      <div className='grid min-w-0 gap-6'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('rankings.backToSite')}
          </DetailBackLink>
        )}
        <section
          className='border-primary/30 bg-primary/5 rounded-lg border p-4'
          role='note'
        >
          {t('rankings.localBoundary')}
        </section>
        <div
          className='flex flex-wrap gap-2'
          role='tablist'
          aria-label={t('rankings.tabs.label')}
        >
          {(['models', 'vendors'] as const).map((tab) => (
            <Button
              aria-selected={search.tab === tab}
              key={tab}
              onClick={() => onSearchChange({ tab })}
              role='tab'
              variant={search.tab === tab ? 'secondary' : 'outline'}
            >
              {tab === 'models'
                ? t('rankings.tabs.models')
                : t('rankings.tabs.vendors')}
            </Button>
          ))}
        </div>
        <section className='border-border grid gap-3 rounded-lg border p-4'>
          <div className='flex flex-wrap gap-2'>
            {periods.map((period) => (
              <Button
                aria-pressed={search.period === period}
                key={period}
                onClick={() => onSearchChange({ period })}
                variant={search.period === period ? 'secondary' : 'outline'}
              >
                {periodText(period, t)}
              </Button>
            ))}
          </div>
          {!siteId && (
            <label className='grid gap-1 text-sm'>
              <span>{t('rankings.siteIds')}</span>
              <Input
                inputMode='numeric'
                onChange={(event) =>
                  onSearchChange({
                    siteIds: event.target.value
                      .split(',')
                      .map((x) => x.trim())
                      .filter(isIdString)
                      .map(parseIdString),
                  })
                }
                value={search.siteIds.join(',')}
              />
            </label>
          )}
        </section>
        {data && (
          <>
            <div className='flex flex-wrap items-center gap-2' role='status'>
              <DataStatusBadge status={data.data_status} />
              <span>
                {t('rankings.range', {
                  end: time(data.end_timestamp),
                  start: time(data.start_timestamp),
                })}
              </span>
              <span>{t('rankings.asOf', { time: time(data.as_of) })}</span>
            </div>
            <DataTable
              ariaLabel={t('rankings.table')}
              columns={columns}
              data={data.items}
              emptyTitle={t('rankings.empty')}
              error={rankingQuery.isError}
              loading={rankingQuery.isPending}
              onRetry={() => void rankingQuery.refetch()}
              renderMobileCard={(item) => (
                <article className='border-border grid gap-2 rounded-lg border p-4'>
                  <div className='flex justify-between gap-2'>
                    <span className='font-medium'>
                      #{item.rank} {name(item, vendors, t)}
                    </span>
                    <MetricValue value={item.growth} />
                  </div>
                  <span>
                    {t('rankings.tokensValue', { value: item.token_used })}
                  </span>
                  <span>
                    {t('rankings.requestsValue', {
                      value: item.request_count,
                    })}
                  </span>
                  <span>{t('rankings.quotaValue', { value: item.quota })}</span>
                  <span>{t('rankings.shareValue', { value: item.share })}</span>
                </article>
              )}
            />
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
            <section className='grid gap-2'>
              <h3 className='font-semibold'>{t('rankings.history')}</h3>
              <div
                aria-label={t('rankings.history')}
                className='overflow-x-auto'
                tabIndex={0}
              >
                <table className='w-full min-w-xl text-sm'>
                  <thead>
                    <tr>
                      <th>{t('rankings.bucket')}</th>
                      <th>{t('rankings.dimension')}</th>
                      <th>{t('rankings.tokens')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.history.map((point) => (
                      <tr
                        className='border-t'
                        key={`${point.bucket_start}:${point.dimension_id}`}
                      >
                        <td>{time(point.bucket_start)}</td>
                        <td>{point.dimension_id}</td>
                        <td>{point.token_used}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
            <section className='grid gap-2'>
              <h3 className='font-semibold'>{t('rankings.siteBreakdown')}</h3>
              {data.site_breakdown.map((item) => (
                <article
                  className='border-border flex flex-wrap justify-between gap-2 rounded-lg border p-3'
                  key={`${item.site_id}:${item.dimension_id}`}
                >
                  <span>
                    {t('rankings.siteBreakdownIdentity', {
                      dimension: item.dimension_id,
                      id: item.site_id,
                      name: item.site_name,
                    })}
                  </span>
                  <span>{item.token_used}</span>
                  <DataStatusBadge status={item.data_status} />
                  <span>{time(item.as_of)}</span>
                </article>
              ))}
            </section>
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
