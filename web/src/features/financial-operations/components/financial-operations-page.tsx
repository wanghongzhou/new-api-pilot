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
import { useMemo, useState, type ChangeEvent } from 'react'
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
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import {
  isIdString,
  isNonNegativeIdString,
  parseIdString,
  parseNonNegativeIdString,
} from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'
import { hasFilterChanges } from '@/lib/filter-state'

import {
  getRedemptionStatistics,
  getSiteRedemptionStatistics,
  getSiteTopupStatistics,
  getTopupStatistics,
  listRedemptions,
  listSiteRedemptions,
  listSiteTopups,
  listTopups,
} from '../api'
import { buildFinancialOperationsExportRequest } from '../export-request'
import { financialOperationsKeys } from '../query-keys'
import {
  buildFinancialOperationsSearch,
  type FinancialOperationsSearch,
} from '../search'
import type {
  FinanceBreakdown,
  FinanceInventoryPage,
  FinanceInventoryQueryParams,
  FinanceMetric,
  FinanceRemoteState,
  FinanceStatisticsResponse,
  RedemptionInventoryItem,
  TopupInventoryItem,
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

function purposeText(
  search: FinancialOperationsSearch,
  t: (key: string) => string
) {
  if (search.tab === 'topups' && search.view === 'analysis') {
    return {
      description: t('financialOperations.purpose.topupsAnalysisDescription'),
      title: t('financialOperations.purpose.topupsAnalysisTitle'),
    }
  }
  if (search.tab === 'topups') {
    return {
      description: t('financialOperations.purpose.topupsListDescription'),
      title: t('financialOperations.purpose.topupsListTitle'),
    }
  }
  if (search.view === 'analysis') {
    return {
      description: t(
        'financialOperations.purpose.redemptionsAnalysisDescription'
      ),
      title: t('financialOperations.purpose.redemptionsAnalysisTitle'),
    }
  }
  return {
    description: t('financialOperations.purpose.redemptionsListDescription'),
    title: t('financialOperations.purpose.redemptionsListTitle'),
  }
}

function queryParams(search: FinancialOperationsSearch) {
  return {
    end_timestamp: search.end,
    keyword:
      search.tab === 'redemptions' ? search.keyword || undefined : undefined,
    methods: search.tab === 'topups' ? search.methods : undefined,
    p: search.page,
    page_size: search.pageSize,
    providers: search.tab === 'topups' ? search.providers : undefined,
    remote_id: search.remoteId,
    remote_user_id: search.remoteUserId,
    site_ids: search.siteIds,
    start_timestamp: search.start,
    states: search.states,
    statuses: search.statuses,
  } satisfies FinanceInventoryQueryParams
}

function RemoteStateBadge({ state }: { state: FinanceRemoteState }) {
  const { t } = useTranslation()
  return (
    <Badge variant={state === 'normal' ? 'success' : 'warning'}>
      {state === 'normal'
        ? t('financialOperations.state.normal')
        : t('financialOperations.state.missing')}
    </Badge>
  )
}

function Filters({
  global,
  onChange,
  search,
  sites,
}: {
  global: boolean
  onChange: (changes: Partial<FinancialOperationsSearch>) => void
  search: FinancialOperationsSearch
  sites: SiteListItem[]
}) {
  const { t } = useTranslation()
  const listChange =
    (key: 'methods' | 'providers' | 'statuses') =>
    (event: ChangeEvent<HTMLInputElement>) =>
      onChange({
        [key]: event.target.value
          .split(',')
          .map((value) => value.trim())
          .filter(Boolean),
        page: 1,
      })
  const reset = buildFinancialOperationsSearch({
    pageSize: search.pageSize,
    tab: search.tab,
    view: search.view,
  })
  const advancedCount = [
    search.remoteUserId != null,
    search.statuses.length > 0,
    search.providers.length > 0,
    search.methods.length > 0,
    search.start !== reset.start,
    search.end !== reset.end,
  ].filter(Boolean).length
  return (
    <FilterPanel
      advanced={
        <>
          <label className='grid gap-1 text-sm'>
            <span>{t('financialOperations.filters.remoteUserId')}</span>
            <Input
              inputMode='numeric'
              onChange={(event) =>
                onChange({
                  page: 1,
                  remoteUserId: isNonNegativeIdString(event.target.value)
                    ? parseNonNegativeIdString(event.target.value)
                    : undefined,
                })
              }
              value={search.remoteUserId ?? ''}
            />
          </label>
          <label className='grid gap-1 text-sm'>
            <span>{t('financialOperations.filters.statuses')}</span>
            <Input
              onChange={listChange('statuses')}
              value={search.statuses.join(',')}
            />
          </label>
          {search.tab === 'topups' && (
            <>
              <label className='grid gap-1 text-sm'>
                <span>{t('financialOperations.filters.providers')}</span>
                <Input
                  onChange={listChange('providers')}
                  value={search.providers.join(',')}
                />
              </label>
              <label className='grid gap-1 text-sm'>
                <span>{t('financialOperations.filters.methods')}</span>
                <Input
                  onChange={listChange('methods')}
                  value={search.methods.join(',')}
                />
              </label>
            </>
          )}
          <label className='grid gap-1 text-sm'>
            <span>{t('financialOperations.filters.start')}</span>
            <Input
              onChange={(event) => {
                const start = parseDateTime(event.target.value)
                if (start != null) onChange({ page: 1, start })
              }}
              type='datetime-local'
              value={dateTimeValue(search.start)}
            />
          </label>
          <label className='grid gap-1 text-sm'>
            <span>{t('financialOperations.filters.end')}</span>
            <Input
              onChange={(event) => {
                const end = parseDateTime(event.target.value)
                if (end != null) onChange({ end, page: 1 })
              }}
              type='datetime-local'
              value={dateTimeValue(search.end)}
            />
          </label>
        </>
      }
      advancedCount={advancedCount}
      advancedMode='popover'
      description={t('financialOperations.filters.description')}
      hasActiveFilters={hasFilterChanges(search, reset, [
        'end',
        'keyword',
        'methods',
        'providers',
        'remoteId',
        'remoteUserId',
        'siteIds',
        'start',
        'states',
        'statuses',
      ])}
      onReset={() => onChange(reset)}
      title={t('financialOperations.filters.title')}
    >
      <div className='flex min-w-0 flex-1 flex-wrap items-center gap-2'>
        {search.tab === 'redemptions' && (
          <label className='grid gap-1 text-sm'>
            <span>{t('financialOperations.filters.keyword')}</span>
            <Input
              className='min-w-48 sm:w-72'
              onChange={(event) =>
                onChange({ keyword: event.target.value, page: 1 })
              }
              value={search.keyword}
            />
          </label>
        )}
        {global && (
          <FacetedFilter
            clearLabel={t('common.all')}
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
            title={t('financialOperations.filters.siteIds')}
            value={search.siteIds.length === 1 ? search.siteIds[0] : ''}
          />
        )}
        <label className='grid gap-1 text-sm'>
          <span>{t('financialOperations.filters.remoteId')}</span>
          <Input
            inputMode='numeric'
            onChange={(event) =>
              onChange({
                page: 1,
                remoteId: isIdString(event.target.value)
                  ? parseIdString(event.target.value)
                  : undefined,
              })
            }
            value={search.remoteId ?? ''}
          />
        </label>
        <FacetedFilter
          clearLabel={t('common.all')}
          onChange={(value) =>
            onChange({
              page: 1,
              states: value === 'normal' || value === 'missing' ? [value] : [],
            })
          }
          options={[
            { label: t('financialOperations.state.normal'), value: 'normal' },
            { label: t('financialOperations.state.missing'), value: 'missing' },
          ]}
          title={t('financialOperations.filters.states')}
          value={search.states.length === 1 ? search.states[0] : ''}
        />
      </div>
    </FilterPanel>
  )
}

function Summary({ metric, topup }: { metric: FinanceMetric; topup: boolean }) {
  const { t } = useTranslation()
  const items = [
    [t('financialOperations.metric.count'), metric.count],
    [t('financialOperations.metric.missing'), metric.missing_count],
    ...(topup
      ? []
      : [
          [
            t('financialOperations.metric.quota'),
            metric.quota ?? null,
          ] as const,
        ]),
  ] as const
  return (
    <div className='grid gap-3 sm:grid-cols-3'>
      {items.map(([label, value]) => (
        <div
          className='bg-card text-card-foreground ring-foreground/10 flex min-w-0 items-center gap-3 rounded-xl p-4 ring-1'
          key={label}
        >
          <span className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg'>
            <HugeiconsIcon icon={Database01Icon} size={18} strokeWidth={2} />
          </span>
          <dl className='min-w-0'>
            <dt className='text-muted-foreground text-xs'>{label}</dt>
            <dd className='mt-0.5 text-2xl font-semibold tracking-tight break-all'>
              <MetricValue value={value} />
            </dd>
          </dl>
        </div>
      ))}
    </div>
  )
}

function Breakdown({
  items,
  nominal,
  title,
}: {
  items: FinanceBreakdown[]
  nominal: boolean
  title: string
}) {
  const { t } = useTranslation()
  return (
    <section className='grid gap-3'>
      <h2 className='text-lg font-semibold'>{title}</h2>
      {items.length === 0 ? (
        <p className='text-muted-foreground text-sm'>
          {t('financialOperations.breakdown.empty')}
        </p>
      ) : (
        <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-3'>
          {items.map((item) => (
            <article
              className='bg-card text-card-foreground ring-foreground/10 grid gap-2 rounded-xl p-4 ring-1'
              key={`${item.site_id}:${item.dimension_id}:${item.as_of ?? 'na'}:${item.count}:${item.missing_count}`}
            >
              <div className='flex items-start justify-between gap-2'>
                <div>
                  <p className='font-medium break-all'>
                    {item.dimension_name || item.dimension_id}
                  </p>
                  {item.site_name && (
                    <p className='text-muted-foreground text-xs'>
                      {item.site_name} · {item.site_id}
                    </p>
                  )}
                </div>
                <DataStatusBadge status={item.data_status} />
              </div>
              <p className='text-sm'>
                {t('financialOperations.metric.countValue', {
                  value: item.count,
                })}
              </p>
              {nominal && (
                <p className='text-sm break-all'>
                  {t('financialOperations.metric.nominalValue', {
                    amount: item.amount ?? '0',
                    money: item.money ?? '0',
                  })}
                </p>
              )}
              {!nominal && item.quota != null && (
                <p className='text-sm break-all'>
                  {t('financialOperations.metric.quotaValue', {
                    value: item.quota,
                  })}
                </p>
              )}
              <p className='text-muted-foreground text-xs'>
                {t('financialOperations.asOf', { time: timestamp(item.as_of) })}
              </p>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

function TopupTable({
  data,
  error,
  fetching,
  loading,
  onPageChange,
  onPageSizeChange,
  onRetry,
  page,
  pageSize,
}: {
  data?: FinanceInventoryPage<TopupInventoryItem>
  error: boolean
  fetching: boolean
  loading: boolean
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
  onRetry?: () => void
  page: number
  pageSize: number
}) {
  const { t } = useTranslation()
  const columns = useMemo<ColumnDef<TopupInventoryItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div>
            <code>{row.original.remote_id}</code>
            <span className='text-muted-foreground block text-xs'>
              {row.original.site_name} · {row.original.site_id}
            </span>
          </div>
        ),
        header: t('financialOperations.identity'),
        id: 'identity',
      },
      {
        cell: ({ row }) => <code>{row.original.remote_user_id}</code>,
        header: t('financialOperations.remoteUser'),
        id: 'user',
      },
      {
        cell: ({ row }) => (
          <div className='min-w-44'>
            <span className='block break-all'>{row.original.amount}</span>
            <span className='block break-all'>{row.original.money}</span>
            <span className='text-muted-foreground block text-xs'>
              {row.original.payment_provider} / {row.original.payment_method}
            </span>
          </div>
        ),
        header: t('financialOperations.nominal'),
        id: 'nominal',
      },
      { accessorKey: 'status', header: t('common.status') },
      {
        cell: ({ row }) => (
          <RemoteStateBadge state={row.original.remote_state} />
        ),
        header: t('financialOperations.remoteState'),
        id: 'state',
      },
      {
        cell: ({ row }) => (
          <div className='min-w-40 text-xs'>
            <span className='block'>{timestamp(row.original.create_time)}</span>
            <span className='block'>
              {timestamp(row.original.complete_time)}
            </span>
          </div>
        ),
        header: t('financialOperations.timestamps'),
        id: 'timestamps',
      },
    ],
    [t]
  )
  return (
    <DataTable
      ariaLabel={t('financialOperations.topupTable')}
      columns={columns}
      data={data?.items ?? []}
      emptyDescription={t('financialOperations.emptyDescription')}
      emptyTitle={t('financialOperations.empty')}
      error={error}
      fetching={fetching}
      loading={loading}
      onPageChange={onPageChange}
      onPageSizeChange={onPageSizeChange}
      onRetry={onRetry}
      page={page}
      pageSize={pageSize}
      renderMobileCard={(item) => (
        <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
          <div className='flex justify-between gap-2'>
            <div>
              <code>{item.remote_id}</code>
              <p className='text-muted-foreground text-xs'>
                {item.site_name} · {item.site_id}
              </p>
            </div>
            <RemoteStateBadge state={item.remote_state} />
          </div>
          <p className='text-sm'>
            {t('financialOperations.remoteUserValue', {
              value: item.remote_user_id,
            })}
          </p>
          <dl className='grid grid-cols-2 gap-3 text-sm'>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('financialOperations.metric.amount')}
              </dt>
              <dd className='break-all'>{item.amount}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('financialOperations.metric.money')}
              </dt>
              <dd className='break-all'>{item.money}</dd>
            </div>
          </dl>
          <p className='text-xs'>
            {item.payment_provider} / {item.payment_method} · {item.status}
          </p>
          <time className='text-muted-foreground text-xs'>
            {timestamp(item.create_time)}
          </time>
        </article>
      )}
      total={data?.total ?? 0}
    />
  )
}

function RedemptionTable(props: {
  data?: FinanceInventoryPage<RedemptionInventoryItem>
  error: boolean
  fetching: boolean
  loading: boolean
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
  onRetry?: () => void
  page: number
  pageSize: number
}) {
  const { t } = useTranslation()
  const columns = useMemo<ColumnDef<RedemptionInventoryItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div>
            <span className='font-medium break-all'>
              {row.original.name || '-'}
            </span>
            <code className='block text-xs'>{row.original.remote_id}</code>
            <span className='text-muted-foreground block text-xs'>
              {row.original.site_name} · {row.original.site_id}
            </span>
          </div>
        ),
        header: t('financialOperations.identity'),
        id: 'identity',
      },
      {
        cell: ({ row }) => <code>{row.original.remote_user_id}</code>,
        header: t('financialOperations.remoteUser'),
        id: 'user',
      },
      {
        cell: ({ row }) => (
          <span className='break-all'>{row.original.quota}</span>
        ),
        header: t('financialOperations.metric.quota'),
        id: 'quota',
      },
      {
        cell: ({ row }) => (
          <div>
            <Badge
              variant={
                row.original.derived_status === 'expired'
                  ? 'warning'
                  : 'neutral'
              }
            >
              {row.original.derived_status === 'expired'
                ? t('financialOperations.status.expired')
                : row.original.derived_status}
            </Badge>
            <span className='text-muted-foreground block text-xs'>
              {t('financialOperations.rawStatus', {
                value: row.original.status,
              })}
            </span>
          </div>
        ),
        header: t('common.status'),
        id: 'status',
      },
      {
        cell: ({ row }) => (
          <RemoteStateBadge state={row.original.remote_state} />
        ),
        header: t('financialOperations.remoteState'),
        id: 'state',
      },
      {
        cell: ({ row }) => (
          <div className='min-w-40 text-xs'>
            <span className='block'>
              {timestamp(row.original.created_time)}
            </span>
            <span className='block'>
              {timestamp(row.original.redeemed_time)}
            </span>
            <span className='block'>
              {timestamp(row.original.expired_time)}
            </span>
          </div>
        ),
        header: t('financialOperations.timestamps'),
        id: 'timestamps',
      },
    ],
    [t]
  )
  return (
    <DataTable
      ariaLabel={t('financialOperations.redemptionTable')}
      columns={columns}
      data={props.data?.items ?? []}
      emptyDescription={t('financialOperations.emptyDescription')}
      emptyTitle={t('financialOperations.empty')}
      error={props.error}
      fetching={props.fetching}
      loading={props.loading}
      onPageChange={props.onPageChange}
      onPageSizeChange={props.onPageSizeChange}
      onRetry={props.onRetry}
      page={props.page}
      pageSize={props.pageSize}
      renderMobileCard={(item) => (
        <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
          <div className='flex justify-between gap-2'>
            <div>
              <p className='font-medium break-all'>{item.name || '-'}</p>
              <code className='text-xs'>{item.remote_id}</code>
            </div>
            <RemoteStateBadge state={item.remote_state} />
          </div>
          <p className='text-muted-foreground text-xs'>
            {item.site_name} · {item.site_id}
          </p>
          <p className='text-sm break-all'>
            {t('financialOperations.metric.quotaValue', { value: item.quota })}
          </p>
          <div className='flex items-center gap-2'>
            <Badge
              variant={
                item.derived_status === 'expired' ? 'warning' : 'neutral'
              }
            >
              {item.derived_status === 'expired'
                ? t('financialOperations.status.expired')
                : item.derived_status}
            </Badge>
            <code className='text-xs'>{item.used_user_id}</code>
          </div>
          <time className='text-muted-foreground text-xs'>
            {timestamp(item.created_time)}
          </time>
        </article>
      )}
      total={props.data?.total ?? 0}
    />
  )
}

export function FinancialOperationsPage({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<FinancialOperationsSearch>) => void
  search: FinancialOperationsSearch
  siteId?: string
}) {
  const { t } = useTranslation()
  const [initialJob, setInitialJob] = useState<StatisticsExportJobItem>()
  const validSiteId = siteId == null || isIdString(siteId)
  const params = useMemo(() => queryParams(search), [search])
  const siteParams = useMemo(() => ({ page_size: 100 }), [])
  const sitesQuery = useQuery({
    enabled: !siteId,
    queryFn: () => listSites(siteParams),
    queryKey: siteKeys.list(siteParams),
  })
  const topupListQuery = useQuery({
    enabled: validSiteId && search.tab === 'topups' && search.view === 'list',
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? listSiteTopups(parseIdString(siteId), params)
        : listTopups(params),
    queryKey: financialOperationsKeys.list('topups', siteId, params),
  })
  const redemptionListQuery = useQuery({
    enabled:
      validSiteId && search.tab === 'redemptions' && search.view === 'list',
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? listSiteRedemptions(parseIdString(siteId), params)
        : listRedemptions(params),
    queryKey: financialOperationsKeys.list('redemptions', siteId, params),
  })
  const statisticsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () => {
      if (search.tab === 'topups') {
        return siteId && isIdString(siteId)
          ? getSiteTopupStatistics(parseIdString(siteId), params)
          : getTopupStatistics(params)
      }
      return siteId && isIdString(siteId)
        ? getSiteRedemptionStatistics(parseIdString(siteId), params)
        : getRedemptionStatistics(params)
    },
    queryKey: financialOperationsKeys.statistics(search.tab, siteId, params),
  })
  const exportMutation = useMutation({
    mutationFn: (format: StatisticsExportFormat) =>
      createStatisticsExport(
        buildFinancialOperationsExportRequest(
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
  const statistics = statisticsQuery.data as
    | FinanceStatisticsResponse
    | undefined
  const activeListQuery =
    search.tab === 'topups' ? topupListQuery : redemptionListQuery
  const topupData = search.tab === 'topups' ? topupListQuery.data : undefined
  const redemptionData =
    search.tab === 'redemptions' ? redemptionListQuery.data : undefined
  const currentPage = topupData ?? redemptionData
  const completeness =
    search.view === 'list'
      ? currentPage?.completeness
      : statistics?.completeness
  const purpose = purposeText(search, t)

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
                {t('financialOperations.export', {
                  format: format.toUpperCase(),
                })}
              </Button>
            ))
          : undefined
      }
      description={
        siteId
          ? t('financialOperations.siteDescription', { id: siteId })
          : t('financialOperations.description')
      }
      fixedContent
      title={
        siteId
          ? t('financialOperations.siteTitle')
          : t('financialOperations.title')
      }
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('financialOperations.backToSite')}
          </DetailBackLink>
        )}
        {siteId && (
          <OperationsAnalyticsNavigation active='financial' siteId={siteId} />
        )}
        {statistics && (
          <Summary
            metric={statistics.summary}
            topup={search.tab === 'topups'}
          />
        )}
        <Tabs
          onValueChange={(value) => {
            const [tab, view] = value.split(':') as [
              FinancialOperationsSearch['tab'],
              FinancialOperationsSearch['view'],
            ]
            onSearchChange({
              page: 1,
              tab,
              view,
            })
          }}
          value={`${search.tab}:${search.view}`}
        >
          <TabsList aria-label={t('financialOperations.tabs.label')}>
            <TabsTrigger value='topups:list'>
              {t('financialOperations.tabs.topupList')}
            </TabsTrigger>
            <TabsTrigger value='topups:analysis'>
              {t('financialOperations.tabs.topupAnalysis')}
            </TabsTrigger>
            <TabsTrigger value='redemptions:list'>
              {t('financialOperations.tabs.redemptionList')}
            </TabsTrigger>
            <TabsTrigger value='redemptions:analysis'>
              {t('financialOperations.tabs.redemptionAnalysis')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <OperationsViewPurpose
          badges={
            <>
              {statistics && (
                <span className='inline-flex items-center gap-1.5 text-xs'>
                  <span className='text-muted-foreground'>
                    {t('financialOperations.statisticsStatus')}
                  </span>
                  <DataStatusBadge status={statistics.data_status} />
                </span>
              )}
              {search.view === 'list' && currentPage && (
                <span className='inline-flex items-center gap-1.5 text-xs'>
                  <span className='text-muted-foreground'>
                    {t('financialOperations.listStatus')}
                  </span>
                  <DataStatusBadge status={currentPage.data_status} />
                </span>
              )}
              {completeness && (
                <Badge variant='outline'>
                  {t('financialOperations.completeness', {
                    complete: completeness.complete_site_count,
                    expected: completeness.expected_site_count,
                    pending: completeness.pending_site_count,
                    unavailable: completeness.unavailable_site_count,
                  })}
                </Badge>
              )}
            </>
          }
          description={purpose.description}
          icon={search.view === 'list' ? Database01Icon : Chart01Icon}
          notice={
            search.tab === 'topups'
              ? t('financialOperations.nominalNotice.description')
              : t('financialOperations.security.description')
          }
          title={purpose.title}
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
        {search.view === 'list' && activeListQuery.isError && currentPage && (
          <QueryStateAlert
            message={t('operationsAnalytics.staleListData')}
            onRetry={() => void activeListQuery.refetch()}
          />
        )}
        {statisticsQuery.isError && statistics && (
          <QueryStateAlert
            message={t('operationsAnalytics.staleStatisticsData')}
            onRetry={() => void statisticsQuery.refetch()}
          />
        )}
        {search.view === 'list' &&
          (search.tab === 'topups' ? (
            <TopupTable
              data={topupData}
              error={!validSiteId || activeListQuery.isError}
              fetching={activeListQuery.isFetching}
              loading={activeListQuery.isPending}
              onPageChange={(page) => onSearchChange({ page })}
              onPageSizeChange={(pageSize) =>
                onSearchChange({ page: 1, pageSize })
              }
              onRetry={
                validSiteId ? () => void activeListQuery.refetch() : undefined
              }
              page={search.page}
              pageSize={search.pageSize}
            />
          ) : (
            <RedemptionTable
              data={redemptionData}
              error={!validSiteId || activeListQuery.isError}
              fetching={activeListQuery.isFetching}
              loading={activeListQuery.isPending}
              onPageChange={(page) => onSearchChange({ page })}
              onPageSizeChange={(pageSize) =>
                onSearchChange({ page: 1, pageSize })
              }
              onRetry={
                validSiteId ? () => void activeListQuery.refetch() : undefined
              }
              page={search.page}
              pageSize={search.pageSize}
            />
          ))}
        {statisticsQuery.isError && !statistics && (
          <ErrorState
            className='min-h-40'
            onRetry={() => void statisticsQuery.refetch()}
            title={t('financialOperations.statisticsError')}
          />
        )}
        {statistics && search.view === 'analysis' && (
          <div className='min-h-0 flex-1 overflow-y-auto pr-1' tabIndex={0}>
            <Breakdown
              items={statistics.status_breakdown}
              nominal={false}
              title={t('financialOperations.breakdown.status')}
            />
            {search.tab === 'topups' && (
              <Breakdown
                items={statistics.provider_breakdown ?? []}
                nominal
                title={t('financialOperations.breakdown.provider')}
              />
            )}
            <Breakdown
              items={statistics.site_breakdown}
              nominal={search.tab === 'topups'}
              title={t('financialOperations.breakdown.site')}
            />
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
