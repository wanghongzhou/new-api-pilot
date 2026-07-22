import { ArrowLeft01Icon, FileExportIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataStatusBadge } from '@/components/data/data-status'
import { FilterPanel } from '@/components/data/filter-panel'
import { MetricValue } from '@/components/data/metric-value'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
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
}: {
  global: boolean
  onChange: (changes: Partial<FinancialOperationsSearch>) => void
  search: FinancialOperationsSearch
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
  return (
    <FilterPanel
      description={t('financialOperations.filters.description')}
      onReset={() =>
        onChange(
          buildFinancialOperationsSearch({
            pageSize: search.pageSize,
            tab: search.tab,
          })
        )
      }
      title={t('financialOperations.filters.title')}
    >
      <div className='grid min-w-0 flex-1 gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        {global && (
          <label className='grid gap-1 text-sm'>
            <span>{t('financialOperations.filters.siteIds')}</span>
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
              value={search.siteIds.join(',')}
            />
          </label>
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
        {search.tab === 'topups' ? (
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
        ) : (
          <label className='grid gap-1 text-sm'>
            <span>{t('financialOperations.filters.keyword')}</span>
            <Input
              onChange={(event) =>
                onChange({ keyword: event.target.value, page: 1 })
              }
              value={search.keyword}
            />
          </label>
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
      </div>
      <fieldset className='grid gap-2'>
        <legend className='text-sm'>
          {t('financialOperations.filters.states')}
        </legend>
        <div className='flex flex-wrap gap-2'>
          {(['normal', 'missing'] as const).map((state) => {
            const active = search.states.includes(state)
            return (
              <Button
                aria-pressed={active}
                key={state}
                onClick={() =>
                  onChange({
                    page: 1,
                    states: active
                      ? search.states.filter((value) => value !== state)
                      : [...search.states, state],
                  })
                }
                size='sm'
                type='button'
                variant={active ? 'secondary' : 'outline'}
              >
                {state === 'normal'
                  ? t('financialOperations.state.normal')
                  : t('financialOperations.state.missing')}
              </Button>
            )
          })}
        </div>
      </fieldset>
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
    <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-3'>
      {items.map(([label, value]) => (
        <div className='border-border min-w-0 border-b p-4' key={label}>
          <dt className='text-muted-foreground text-xs'>{label}</dt>
          <dd className='mt-1 text-xl font-semibold break-all'>
            <MetricValue value={value} />
          </dd>
        </div>
      ))}
    </dl>
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
              className='border-border bg-card grid gap-2 rounded-lg border p-4'
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
                    amount: item.amount ?? '-',
                    money: item.money ?? '-',
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
  onRetry: () => void
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
        <article className='border-border bg-card grid gap-3 rounded-lg border p-4'>
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
  onRetry: () => void
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
        <article className='border-border bg-card grid gap-3 rounded-lg border p-4'>
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
  const topupListQuery = useQuery({
    enabled: validSiteId && search.tab === 'topups',
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? listSiteTopups(parseIdString(siteId), params)
        : listTopups(params),
    queryKey: financialOperationsKeys.list('topups', siteId, params),
  })
  const redemptionListQuery = useQuery({
    enabled: validSiteId && search.tab === 'redemptions',
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
          {t('financialOperations.export', { format: format.toUpperCase() })}
        </Button>
      ))}
      description={
        siteId
          ? t('financialOperations.siteDescription', { id: siteId })
          : t('financialOperations.description')
      }
      title={
        siteId
          ? t('financialOperations.siteTitle')
          : t('financialOperations.title')
      }
    >
      <div className='grid min-w-0 gap-6'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('financialOperations.backToSite')}
          </DetailBackLink>
        )}
        <section
          className='border-destructive/20 bg-muted/30 rounded-lg border p-4'
          role='note'
        >
          <p className='font-medium'>
            {t('financialOperations.security.title')}
          </p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('financialOperations.security.description')}
          </p>
        </section>
        <Tabs
          onValueChange={(tab) =>
            onSearchChange({
              page: 1,
              tab: tab as FinancialOperationsSearch['tab'],
            })
          }
          value={search.tab}
        >
          <TabsList aria-label={t('financialOperations.tabs.label')}>
            <TabsTrigger value='topups'>
              {t('financialOperations.tabs.topups')}
            </TabsTrigger>
            <TabsTrigger value='redemptions'>
              {t('financialOperations.tabs.redemptions')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        {search.tab === 'topups' && (
          <section
            className='border-warning/30 bg-warning/5 rounded-lg border p-4'
            role='note'
          >
            <p className='font-medium'>
              {t('financialOperations.nominalNotice.title')}
            </p>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('financialOperations.nominalNotice.description')}
            </p>
          </section>
        )}
        <Filters global={!siteId} onChange={onSearchChange} search={search} />
        <div className='grid gap-3 sm:grid-cols-2'>
          {currentPage && (
            <section
              className='border-border flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3'
              role='status'
            >
              <div className='flex items-center gap-2'>
                <span className='text-sm font-medium'>
                  {t('financialOperations.listStatus')}
                </span>
                <DataStatusBadge status={currentPage.data_status} />
              </div>
              <span className='text-muted-foreground text-xs'>
                {t('financialOperations.asOf', {
                  time: timestamp(currentPage.as_of),
                })}
              </span>
            </section>
          )}
          {statistics && (
            <section
              className='border-border flex items-center gap-2 rounded-lg border p-3'
              role='status'
            >
              <span className='text-sm font-medium'>
                {t('financialOperations.statisticsStatus')}
              </span>
              <DataStatusBadge status={statistics.data_status} />
            </section>
          )}
        </div>
        {statistics && (
          <Summary
            metric={statistics.summary}
            topup={search.tab === 'topups'}
          />
        )}
        {search.tab === 'topups' ? (
          <TopupTable
            data={topupData}
            error={!validSiteId || activeListQuery.isError}
            fetching={activeListQuery.isFetching}
            loading={activeListQuery.isPending}
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={() => void activeListQuery.refetch()}
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
            onRetry={() => void activeListQuery.refetch()}
            page={search.page}
            pageSize={search.pageSize}
          />
        )}
        {statisticsQuery.isError && !statistics && (
          <section className='border-destructive/30 bg-destructive/5 rounded-lg border p-4'>
            <p>{t('financialOperations.statisticsError')}</p>
            <Button
              className='mt-3'
              onClick={() => void statisticsQuery.refetch()}
              variant='outline'
            >
              {t('common.retry')}
            </Button>
          </section>
        )}
        {statistics && (
          <>
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
