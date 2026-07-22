import { ArrowLeft01Icon, FileExportIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'
import { useMemo, useState } from 'react'
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
import { createStatisticsExport } from '@/features/statistics/api'
import { ExportTaskSheet } from '@/features/statistics/components/export-task-sheet'
import type {
  StatisticsExportFormat,
  StatisticsExportJobItem,
} from '@/features/statistics/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { isIdString, parseIdString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'

import {
  getSiteSubscriptionPlanStatistics,
  getSubscriptionPlanStatistics,
  listSiteSubscriptionPlans,
  listSubscriptionPlans,
} from '../api'
import { buildSubscriptionPlanExportRequest } from '../export-request'
import { subscriptionPlanKeys } from '../query-keys'
import {
  buildSubscriptionPlanSearch,
  type SubscriptionPlanSearch,
} from '../search'
import type {
  SubscriptionDurationUnit,
  SubscriptionPlanBreakdown,
  SubscriptionPlanItem,
  SubscriptionPlanQueryParams,
  SubscriptionPlanStatistics,
  SubscriptionPlanState,
  SubscriptionResetPeriod,
} from '../types'

function params(search: SubscriptionPlanSearch): SubscriptionPlanQueryParams {
  return {
    enabled: search.enabled,
    keyword: search.keyword || undefined,
    p: search.page,
    page_size: search.pageSize,
    site_ids: search.siteIds,
    states: search.states,
  }
}

function timestamp(value: number | null) {
  return value == null || value <= 0
    ? '-'
    : fromUnixSeconds(value).format('YYYY-MM-DD HH:mm:ss')
}

function durationUnitText(unit: SubscriptionDurationUnit, t: TFunction) {
  switch (unit) {
    case 'year':
      return t('subscriptionPlans.duration.year')
    case 'month':
      return t('subscriptionPlans.duration.month')
    case 'day':
      return t('subscriptionPlans.duration.day')
    case 'hour':
      return t('subscriptionPlans.duration.hour')
    case 'custom':
      return t('subscriptionPlans.duration.custom')
  }
}

function resetPeriodText(period: SubscriptionResetPeriod, t: TFunction) {
  switch (period) {
    case 'never':
      return t('subscriptionPlans.reset.never')
    case 'daily':
      return t('subscriptionPlans.reset.daily')
    case 'weekly':
      return t('subscriptionPlans.reset.weekly')
    case 'monthly':
      return t('subscriptionPlans.reset.monthly')
    case 'custom':
      return t('subscriptionPlans.reset.custom')
  }
}

function durationText(item: SubscriptionPlanItem, t: TFunction) {
  return item.duration_unit === 'custom'
    ? t('subscriptionPlans.customSecondsValue', {
        value: item.custom_seconds,
      })
    : t('subscriptionPlans.durationValue', {
        unit: durationUnitText(item.duration_unit, t),
        value: item.duration_value,
      })
}

function resetText(item: SubscriptionPlanItem, t: TFunction) {
  return item.quota_reset_period === 'custom'
    ? t('subscriptionPlans.customResetValue', {
        value: item.quota_reset_custom_seconds,
      })
    : resetPeriodText(item.quota_reset_period, t)
}

function EnabledBadge({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation()
  return (
    <Badge variant={enabled ? 'success' : 'neutral'}>
      {enabled
        ? t('subscriptionPlans.enabled')
        : t('subscriptionPlans.disabled')}
    </Badge>
  )
}

function StateBadge({ state }: { state: SubscriptionPlanState }) {
  const { t } = useTranslation()
  return (
    <Badge variant={state === 'normal' ? 'success' : 'warning'}>
      {state === 'normal'
        ? t('subscriptionPlans.state.normal')
        : t('subscriptionPlans.state.missing')}
    </Badge>
  )
}

function Filters({
  global,
  onChange,
  search,
}: {
  global: boolean
  onChange: (changes: Partial<SubscriptionPlanSearch>) => void
  search: SubscriptionPlanSearch
}) {
  const { t } = useTranslation()
  const toggleState = (state: SubscriptionPlanState) => {
    const active = search.states.includes(state)
    onChange({
      page: 1,
      states: active
        ? search.states.filter((value) => value !== state)
        : [...search.states, state],
    })
  }
  return (
    <FilterPanel
      description={t('subscriptionPlans.filters.description')}
      onReset={() =>
        onChange(buildSubscriptionPlanSearch({ pageSize: search.pageSize }))
      }
      title={t('subscriptionPlans.filters.title')}
    >
      <div className='grid min-w-0 flex-1 gap-3 sm:grid-cols-2'>
        <label className='grid gap-1 text-sm'>
          <span>{t('subscriptionPlans.filters.keyword')}</span>
          <Input
            onChange={(event) =>
              onChange({ keyword: event.target.value, page: 1 })
            }
            value={search.keyword}
          />
        </label>
        {global && (
          <label className='grid gap-1 text-sm'>
            <span>{t('subscriptionPlans.filters.siteIds')}</span>
            <Input
              inputMode='numeric'
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
      </div>
      <div className='grid min-w-0 flex-1 gap-3 sm:grid-cols-2'>
        <fieldset className='grid gap-1'>
          <legend className='text-sm'>
            {t('subscriptionPlans.filters.enabled')}
          </legend>
          <div className='flex flex-wrap gap-2'>
            {(
              [
                [undefined, t('subscriptionPlans.filters.all')],
                [true, t('subscriptionPlans.enabled')],
                [false, t('subscriptionPlans.disabled')],
              ] as const
            ).map(([value, label]) => (
              <Button
                aria-pressed={search.enabled === value}
                key={String(value)}
                onClick={() => onChange({ enabled: value, page: 1 })}
                size='sm'
                type='button'
                variant={search.enabled === value ? 'secondary' : 'outline'}
              >
                {label}
              </Button>
            ))}
          </div>
        </fieldset>
        <fieldset className='grid gap-1'>
          <legend className='text-sm'>
            {t('subscriptionPlans.filters.states')}
          </legend>
          <div className='flex flex-wrap gap-2'>
            {(['normal', 'missing'] as const).map((state) => (
              <Button
                aria-pressed={search.states.includes(state)}
                key={state}
                onClick={() => toggleState(state)}
                size='sm'
                type='button'
                variant={
                  search.states.includes(state) ? 'secondary' : 'outline'
                }
              >
                {state === 'normal'
                  ? t('subscriptionPlans.state.normal')
                  : t('subscriptionPlans.state.missing')}
              </Button>
            ))}
          </div>
        </fieldset>
      </div>
    </FilterPanel>
  )
}

function StatisticsGrid({
  values,
}: {
  values: Pick<
    SubscriptionPlanStatistics,
    'disabled' | 'enabled' | 'missing' | 'total'
  >
}) {
  const { t } = useTranslation()
  const items: ReadonlyArray<
    readonly [string, SubscriptionPlanStatistics['total']]
  > = [
    [t('subscriptionPlans.metric.total'), values.total],
    [t('subscriptionPlans.metric.enabled'), values.enabled],
    [t('subscriptionPlans.metric.disabled'), values.disabled],
    [t('subscriptionPlans.metric.missing'), values.missing],
  ]
  return (
    <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 xl:grid-cols-4'>
      {items.map(([label, value]) => (
        <div className='border-border p-4 sm:border-r' key={label}>
          <dt className='text-muted-foreground text-xs'>{label}</dt>
          <dd className='mt-1 text-xl font-semibold'>
            <MetricValue value={value} />
          </dd>
        </div>
      ))}
    </dl>
  )
}

function SiteBreakdown({ items }: { items: SubscriptionPlanBreakdown[] }) {
  const { t } = useTranslation()
  return (
    <section className='grid gap-3'>
      <h2 className='font-semibold'>{t('subscriptionPlans.breakdown.site')}</h2>
      {items.length === 0 ? (
        <p className='text-muted-foreground text-sm'>{t('common.none')}</p>
      ) : (
        <div className='grid gap-2 md:grid-cols-2 xl:grid-cols-3'>
          {items.map((item) => (
            <article
              className='border-border grid gap-2 rounded-lg border p-3'
              key={item.site_id}
            >
              <div className='flex items-start justify-between gap-2'>
                <div>
                  <p className='font-medium'>{item.site_name}</p>
                  <code className='text-muted-foreground text-xs'>
                    {item.site_id}
                  </code>
                </div>
                <DataStatusBadge status={item.data_status} />
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('subscriptionPlans.breakdown.values', {
                  disabled: item.disabled,
                  enabled: item.enabled,
                  missing: item.missing,
                  total: item.total,
                })}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t('subscriptionPlans.asOf', { time: timestamp(item.as_of) })}
              </p>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

export function SubscriptionPlansPage({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<SubscriptionPlanSearch>) => void
  search: SubscriptionPlanSearch
  siteId?: string
}) {
  const { t } = useTranslation()
  const [initialJob, setInitialJob] = useState<StatisticsExportJobItem>()
  const validSiteId = siteId == null || isIdString(siteId)
  const currentParams = useMemo(() => params(search), [search])
  const listQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? listSiteSubscriptionPlans(parseIdString(siteId), currentParams)
        : listSubscriptionPlans(currentParams),
    queryKey:
      siteId && isIdString(siteId)
        ? subscriptionPlanKeys.site(siteId, 'list', currentParams)
        : subscriptionPlanKeys.global('list', currentParams),
  })
  const statisticsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? getSiteSubscriptionPlanStatistics(
            parseIdString(siteId),
            currentParams
          )
        : getSubscriptionPlanStatistics(currentParams),
    queryKey:
      siteId && isIdString(siteId)
        ? subscriptionPlanKeys.site(siteId, 'statistics', currentParams)
        : subscriptionPlanKeys.global('statistics', currentParams),
  })
  const exportMutation = useMutation({
    mutationFn: (format: StatisticsExportFormat) =>
      createStatisticsExport(
        buildSubscriptionPlanExportRequest(
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
  const columns = useMemo<ColumnDef<SubscriptionPlanItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='min-w-44'>
            <p className='font-medium'>{row.original.title}</p>
            <p className='text-muted-foreground text-xs'>
              {row.original.subtitle || '-'}
            </p>
            <p className='text-muted-foreground text-xs'>
              {row.original.site_name} · {row.original.site_id}
            </p>
            <code className='text-muted-foreground text-xs'>
              {row.original.remote_id}
            </code>
          </div>
        ),
        header: t('subscriptionPlans.identity'),
        id: 'identity',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-32 gap-1 text-xs'>
            <strong>
              {row.original.currency} {row.original.price_amount}
            </strong>
            <span>
              {row.original.total_amount === '0'
                ? t('subscriptionPlans.unlimited')
                : t('subscriptionPlans.quotaValue', {
                    value: row.original.total_amount,
                  })}
            </span>
          </div>
        ),
        header: t('subscriptionPlans.priceQuota'),
        id: 'price-quota',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-36 gap-1 text-xs'>
            <span>{durationText(row.original, t)}</span>
            <span>{resetText(row.original, t)}</span>
          </div>
        ),
        header: t('subscriptionPlans.durationReset'),
        id: 'duration-reset',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-28 gap-1'>
            <EnabledBadge enabled={row.original.enabled} />
            <StateBadge state={row.original.remote_state} />
            <DataStatusBadge status={row.original.data_status} />
          </div>
        ),
        header: t('common.status'),
        id: 'status',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-40 gap-1 text-xs'>
            <span>{timestamp(row.original.created_at)}</span>
            <span>{timestamp(row.original.updated_at)}</span>
            <span>
              {t('subscriptionPlans.sortOrderValue', {
                value: row.original.sort_order,
              })}
            </span>
          </div>
        ),
        header: t('subscriptionPlans.timestamps'),
        id: 'timestamps',
      },
    ],
    [t]
  )
  const statistics = statisticsQuery.data
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
          {t('subscriptionPlans.export', { format: format.toUpperCase() })}
        </Button>
      ))}
      description={
        siteId
          ? t('subscriptionPlans.siteDescription', { id: siteId })
          : t('subscriptionPlans.description')
      }
      title={
        siteId ? t('subscriptionPlans.siteTitle') : t('subscriptionPlans.title')
      }
    >
      <div className='grid min-w-0 gap-6'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('subscriptionPlans.backToSite')}
          </DetailBackLink>
        )}
        <section
          className='border-primary/30 bg-primary/5 rounded-lg border p-4'
          role='note'
        >
          <p className='font-medium'>{t('subscriptionPlans.boundary.title')}</p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('subscriptionPlans.boundary.description')}
          </p>
        </section>
        <Filters global={!siteId} onChange={onSearchChange} search={search} />
        {statistics && (
          <div className='grid gap-5'>
            <div className='flex items-center gap-2' role='status'>
              <span>{t('subscriptionPlans.statisticsStatus')}</span>
              <DataStatusBadge status={statistics.data_status} />
            </div>
            <StatisticsGrid values={statistics} />
            <SiteBreakdown items={statistics.site_breakdown} />
          </div>
        )}
        {statisticsQuery.isError && (
          <Button
            onClick={() => void statisticsQuery.refetch()}
            variant='outline'
          >
            {t('common.retry')}
          </Button>
        )}
        <div className='flex items-center gap-2' role='status'>
          <span>{t('subscriptionPlans.listStatus')}</span>
          <DataStatusBadge status={listQuery.data?.data_status ?? 'pending'} />
        </div>
        <DataTable
          ariaLabel={t('subscriptionPlans.table')}
          columns={columns}
          data={listQuery.data?.items ?? []}
          emptyDescription={t('subscriptionPlans.emptyDescription')}
          emptyTitle={t('subscriptionPlans.empty')}
          error={!validSiteId || listQuery.isError}
          fetching={listQuery.isFetching}
          loading={listQuery.isPending}
          onPageChange={(page) => onSearchChange({ page })}
          onPageSizeChange={(pageSize) => onSearchChange({ page: 1, pageSize })}
          onRetry={() => void listQuery.refetch()}
          page={search.page}
          pageSize={search.pageSize}
          renderMobileCard={(item) => (
            <article className='border-border bg-card grid gap-3 rounded-lg border p-4'>
              <div className='flex items-start justify-between gap-2'>
                <div className='min-w-0'>
                  <p className='font-medium'>{item.title}</p>
                  <p className='text-muted-foreground text-xs'>
                    {item.subtitle || '-'}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {item.site_name} · {item.site_id} · {item.remote_id}
                  </p>
                </div>
                <EnabledBadge enabled={item.enabled} />
              </div>
              <strong>
                {item.currency} {item.price_amount}
              </strong>
              <span>
                {item.total_amount === '0'
                  ? t('subscriptionPlans.unlimited')
                  : t('subscriptionPlans.quotaValue', {
                      value: item.total_amount,
                    })}
              </span>
              <span>{durationText(item, t)}</span>
              <span>{resetText(item, t)}</span>
              <div className='flex flex-wrap gap-2'>
                <StateBadge state={item.remote_state} />
                <DataStatusBadge status={item.data_status} />
              </div>
            </article>
          )}
          total={listQuery.data?.total ?? 0}
        />
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
