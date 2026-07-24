import {
  Alert02Icon,
  ArrowLeft01Icon,
  Chart01Icon,
  Database01Icon,
  FileExportIcon,
  Search01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataStatusBadge } from '@/components/data/data-status'
import { FacetedFilter } from '@/components/data/faceted-filter'
import { MetricValue } from '@/components/data/metric-value'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
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
import { isIdString, parseIdString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import { hasFilterChanges } from '@/lib/filter-state'

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
  changeSubscriptionPlanTab,
  hasSubscriptionPlanAnalysisFilters,
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
  sites,
}: {
  global: boolean
  onChange: (changes: Partial<SubscriptionPlanSearch>) => void
  search: SubscriptionPlanSearch
  sites: SiteListItem[]
}) {
  const { t } = useTranslation()
  const setState = (value: string) => {
    onChange({
      page: 1,
      states:
        value === 'normal' || value === 'missing'
          ? [value as SubscriptionPlanState]
          : [],
    })
  }
  const setEnabled = (value: string) => {
    let enabled: boolean | undefined
    if (value === 'enabled') enabled = true
    if (value === 'disabled') enabled = false
    onChange({ enabled, page: 1 })
  }
  let enabledValue = ''
  if (search.enabled === true) enabledValue = 'enabled'
  if (search.enabled === false) enabledValue = 'disabled'
  const reset = buildSubscriptionPlanSearch({ pageSize: search.pageSize })
  const hasActiveFilters = hasFilterChanges(search, reset, [
    'enabled',
    'keyword',
    'siteIds',
    'states',
  ])
  return (
    <section
      aria-label={t('subscriptionPlans.filters.title')}
      className='flex min-w-0 flex-wrap items-center gap-2'
    >
      <label className='relative min-w-48 flex-1 sm:max-w-72'>
        <span className='sr-only'>
          {t('subscriptionPlans.filters.keyword')}
        </span>
        <HugeiconsIcon
          className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2'
          icon={Search01Icon}
          size={15}
          strokeWidth={2}
        />
        <Input
          aria-label={t('subscriptionPlans.filters.keyword')}
          className='h-8 pl-8'
          onChange={(event) =>
            onChange({ keyword: event.target.value, page: 1 })
          }
          placeholder={t('subscriptionPlans.filters.keywordPlaceholder')}
          value={search.keyword}
        />
      </label>
      {global && (
        <FacetedFilter
          clearLabel={t('subscriptionPlans.filters.allSites')}
          onChange={(value) =>
            onChange({
              page: 1,
              siteIds: isIdString(value) ? [parseIdString(value)] : [],
            })
          }
          options={sites.map((site) => ({ label: site.name, value: site.id }))}
          title={t('subscriptionPlans.filters.site')}
          value={search.siteIds.length === 1 ? search.siteIds[0] : ''}
        />
      )}
      <FacetedFilter
        clearLabel={t('subscriptionPlans.filters.all')}
        onChange={setEnabled}
        options={[
          { label: t('subscriptionPlans.enabled'), value: 'enabled' },
          { label: t('subscriptionPlans.disabled'), value: 'disabled' },
        ]}
        title={t('subscriptionPlans.filters.enabled')}
        value={enabledValue}
      />
      <FacetedFilter
        clearLabel={t('subscriptionPlans.filters.allStates')}
        onChange={setState}
        options={[
          { label: t('subscriptionPlans.state.normal'), value: 'normal' },
          { label: t('subscriptionPlans.state.missing'), value: 'missing' },
        ]}
        title={t('subscriptionPlans.filters.states')}
        value={search.states.length === 1 ? search.states[0] : ''}
      />
      {hasActiveFilters && (
        <Button
          className='text-muted-foreground px-2'
          onClick={() => onChange(reset)}
          size='sm'
          type='button'
          variant='ghost'
        >
          {t('common.reset')}
        </Button>
      )}
    </section>
  )
}

function StatisticsGrid({
  values,
}: {
  values?: Pick<
    SubscriptionPlanStatistics,
    'disabled' | 'enabled' | 'missing' | 'total'
  >
}) {
  const { t } = useTranslation()
  const items = [
    {
      icon: Database01Icon,
      label: t('subscriptionPlans.metric.total'),
      value: values?.total,
    },
    {
      icon: Chart01Icon,
      label: t('subscriptionPlans.metric.enabled'),
      value: values?.enabled,
    },
    {
      icon: Chart01Icon,
      label: t('subscriptionPlans.metric.disabled'),
      value: values?.disabled,
    },
    {
      icon: Alert02Icon,
      label: t('subscriptionPlans.metric.missing'),
      value: values?.missing,
    },
  ] as const
  return (
    <dl className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
      {items.map(({ icon, label, value }) => (
        <div
          className='bg-card text-card-foreground ring-foreground/10 flex items-center gap-3 rounded-xl p-4 ring-1'
          key={label}
        >
          <span className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg'>
            <HugeiconsIcon icon={icon} size={18} strokeWidth={2} />
          </span>
          <div>
            <dt className='text-muted-foreground text-xs'>{label}</dt>
            <dd className='mt-0.5 text-2xl font-semibold tracking-tight'>
              {value == null ? '-' : <MetricValue value={value} />}
            </dd>
          </div>
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
  const canonicalizedSearch = useRef(false)
  useEffect(() => {
    if (
      search.tab === 'site-analysis' &&
      hasSubscriptionPlanAnalysisFilters(search)
    ) {
      onSearchChange(changeSubscriptionPlanTab('site-analysis'))
    } else if (!canonicalizedSearch.current) {
      canonicalizedSearch.current = true
      onSearchChange({})
    }
  }, [onSearchChange, search])
  const validSiteId = siteId == null || isIdString(siteId)
  const currentParams = useMemo(() => params(search), [search])
  const overviewParams = useMemo(
    () => params(buildSubscriptionPlanSearch({})),
    []
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
  const listQuery = useQuery({
    enabled: validSiteId && search.tab === 'plans',
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
            overviewParams
          )
        : getSubscriptionPlanStatistics(overviewParams),
    queryKey:
      siteId && isIdString(siteId)
        ? subscriptionPlanKeys.site(siteId, 'statistics', overviewParams)
        : subscriptionPlanKeys.global('statistics', overviewParams),
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
  const listTab = search.tab === 'plans'
  const purpose = listTab
    ? {
        description: t('subscriptionPlans.purpose.description'),
        title: t('subscriptionPlans.purpose.title'),
      }
    : {
        description: t('subscriptionPlans.purpose.siteAnalysis.description'),
        title: t('subscriptionPlans.purpose.siteAnalysis.title'),
      }
  return (
    <SectionPageLayout
      actions={
        listTab
          ? (['xlsx', 'csv'] as const).map((format) => (
              <Button
                disabled={exportMutation.isPending || !validSiteId}
                key={format}
                onClick={() => exportMutation.mutate(format)}
                variant='outline'
              >
                <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
                {t('subscriptionPlans.export', {
                  format: format.toUpperCase(),
                })}
              </Button>
            ))
          : undefined
      }
      description={
        siteId
          ? t('subscriptionPlans.siteDescription', { id: siteId })
          : t('subscriptionPlans.description')
      }
      title={
        siteId ? t('subscriptionPlans.siteTitle') : t('subscriptionPlans.title')
      }
      fixedContent
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('subscriptionPlans.backToSite')}
          </DetailBackLink>
        )}
        <StatisticsGrid values={statistics} />
        <Tabs
          onValueChange={(tab) =>
            onSearchChange(
              changeSubscriptionPlanTab(tab as SubscriptionPlanSearch['tab'])
            )
          }
          value={search.tab}
        >
          <TabsList aria-label={t('subscriptionPlans.tabs.label')}>
            <TabsTrigger value='plans'>
              <HugeiconsIcon icon={Database01Icon} size={15} strokeWidth={2} />
              {t('subscriptionPlans.tabs.plans')}
              {statistics && (
                <Badge className='px-1.5 font-mono' variant='secondary'>
                  {statistics.total}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value='site-analysis'>
              <HugeiconsIcon icon={Chart01Icon} size={15} strokeWidth={2} />
              {t('subscriptionPlans.tabs.siteAnalysis')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <section className='border-border bg-muted/30 flex items-start gap-3 rounded-xl border p-4'>
          <span className='bg-background text-muted-foreground ring-foreground/10 flex size-9 shrink-0 items-center justify-center rounded-lg ring-1'>
            <HugeiconsIcon
              icon={listTab ? Database01Icon : Chart01Icon}
              size={18}
              strokeWidth={2}
            />
          </span>
          <div className='min-w-0 flex-1'>
            <div className='flex flex-wrap items-center gap-2'>
              <h2 className='font-medium'>{purpose.title}</h2>
              {(listQuery.data?.data_status ?? statistics?.data_status) && (
                <DataStatusBadge
                  status={
                    (listTab
                      ? listQuery.data?.data_status
                      : statistics?.data_status) ?? 'pending'
                  }
                />
              )}
            </div>
            <p className='text-muted-foreground mt-1 text-sm'>
              {purpose.description}
            </p>
          </div>
        </section>
        {listTab && (
          <Filters
            global={!siteId}
            onChange={onSearchChange}
            search={search}
            sites={sitesQuery.data?.items ?? []}
          />
        )}
        {statisticsQuery.isError && (
          <Button
            onClick={() => void statisticsQuery.refetch()}
            variant='outline'
          >
            {t('common.retry')}
          </Button>
        )}
        {listTab && (
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
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={() => void listQuery.refetch()}
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(item) => (
              <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
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
        )}
        {!listTab && (
          <div className='min-h-0 flex-1 overflow-y-auto'>
            {statistics && <SiteBreakdown items={statistics.site_breakdown} />}
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
