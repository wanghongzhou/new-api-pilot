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
import { formatNumericDisplayValue } from '@/lib/display-value'
import { hasFilterChanges } from '@/lib/filter-state'

import {
  getPricingCatalogStatistics,
  getSitePricingCatalogStatistics,
  listPricingCatalog,
  listPricingGroups,
  listSitePricingCatalog,
  listSitePricingGroups,
} from '../api'
import { buildPricingGroupExportRequest } from '../export-request'
import { pricingGroupKeys } from '../query-keys'
import {
  buildPricingGroupSearch,
  changePricingGroupTab,
  hasPricingAnalysisFilters,
  isPricingAnalysisTab,
  type PricingGroupSearch,
} from '../search'
import type {
  PricingCatalogTab,
  PricingCatalogItem,
  PricingCatalogQueryParams,
  PricingCatalogSiteBreakdown,
  PricingCatalogStatistics,
  PricingCatalogState,
  PricingGroupItem,
} from '../types'

function params(search: PricingGroupSearch): PricingCatalogQueryParams {
  return {
    group: search.group || undefined,
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

function StateBadge({ state }: { state: PricingCatalogState }) {
  const { t } = useTranslation()
  return (
    <Badge variant={state === 'normal' ? 'success' : 'warning'}>
      {state === 'normal'
        ? t('pricingGroups.state.normal')
        : t('pricingGroups.state.missing')}
    </Badge>
  )
}

function VisibilityBadge({ visible }: { visible: boolean }) {
  const { t } = useTranslation()
  return (
    <Badge variant={visible ? 'success' : 'neutral'}>
      {visible
        ? t('pricingGroups.visibility.root')
        : t('pricingGroups.visibility.restricted')}
    </Badge>
  )
}

function TextBadges({ values }: { values: string[] }) {
  const { t } = useTranslation()
  if (values.length === 0) {
    return <span className='text-muted-foreground'>{t('common.none')}</span>
  }
  return (
    <div className='flex max-w-72 flex-wrap gap-1'>
      {values.map((value) => (
        <Badge key={value} variant='neutral'>
          {value}
        </Badge>
      ))}
    </div>
  )
}

function Filters({
  global,
  onChange,
  search,
  sites,
}: {
  global: boolean
  onChange: (changes: Partial<PricingGroupSearch>) => void
  search: PricingGroupSearch
  sites: SiteListItem[]
}) {
  const { t } = useTranslation()
  const setState = (value: string) =>
    onChange({
      page: 1,
      states:
        value === 'normal' || value === 'missing'
          ? [value as PricingCatalogState]
          : [],
    })
  const reset = buildPricingGroupSearch({
    pageSize: search.pageSize,
    tab: search.tab,
  })
  const hasActiveFilters = hasFilterChanges(search, reset, [
    'group',
    'keyword',
    'siteIds',
    'states',
  ])
  return (
    <section
      aria-label={t('pricingGroups.filters.title')}
      className='flex min-w-0 flex-wrap items-center gap-2'
    >
      <label className='relative min-w-48 flex-1 sm:max-w-72'>
        <span className='sr-only'>{t('pricingGroups.filters.keyword')}</span>
        <HugeiconsIcon
          className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2'
          icon={Search01Icon}
          size={15}
          strokeWidth={2}
        />
        <Input
          aria-label={t('pricingGroups.filters.keyword')}
          className='h-8 pl-8'
          onChange={(event) =>
            onChange({ keyword: event.target.value, page: 1 })
          }
          placeholder={
            search.tab === 'pricing'
              ? t('pricingGroups.filters.modelPlaceholder')
              : t('pricingGroups.filters.groupPlaceholder')
          }
          value={search.keyword}
        />
      </label>
      {search.tab === 'pricing' && (
        <Input
          aria-label={t('pricingGroups.filters.group')}
          className='h-8 w-36'
          onChange={(event) => onChange({ group: event.target.value, page: 1 })}
          placeholder={t('pricingGroups.filters.group')}
          value={search.group}
        />
      )}
      {global && (
        <FacetedFilter
          clearLabel={t('pricingGroups.filters.allSites')}
          onChange={(value) =>
            onChange({
              page: 1,
              siteIds: isIdString(value) ? [parseIdString(value)] : [],
            })
          }
          options={sites.map((site) => ({ label: site.name, value: site.id }))}
          title={t('pricingGroups.filters.site')}
          value={search.siteIds.length === 1 ? search.siteIds[0] : ''}
        />
      )}
      <FacetedFilter
        clearLabel={t('pricingGroups.filters.allStates')}
        onChange={setState}
        options={[
          { label: t('pricingGroups.state.normal'), value: 'normal' },
          { label: t('pricingGroups.state.missing'), value: 'missing' },
        ]}
        title={t('pricingGroups.filters.states')}
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

function SiteBreakdown({
  items,
  kind,
}: {
  items: PricingCatalogSiteBreakdown[]
  kind: 'groups' | 'pricing'
}) {
  const { t } = useTranslation()
  return (
    <section className='grid gap-3'>
      <h2 className='font-semibold'>
        {kind === 'pricing'
          ? t('pricingGroups.breakdown.pricingSite')
          : t('pricingGroups.breakdown.groupSite')}
      </h2>
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
                {kind === 'pricing'
                  ? t('pricingGroups.breakdown.values', {
                      missing: item.missing,
                      total: item.total,
                    })
                  : t('pricingGroups.breakdown.groupSiteValues', {
                      missing: item.missing,
                      total: item.total,
                    })}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t('pricingGroups.asOf', { time: timestamp(item.as_of) })}
              </p>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

function TypedBreakdown({
  tab,
  statistics,
}: {
  tab: PricingCatalogTab
  statistics: Awaited<ReturnType<typeof getPricingCatalogStatistics>>
}) {
  const { t } = useTranslation()
  if (tab === 'vendor-analysis') {
    return (
      <section className='grid content-start gap-2'>
        <h2 className='font-semibold'>{t('pricingGroups.breakdown.vendor')}</h2>
        {statistics.vendor_breakdown.length === 0 && (
          <p className='text-muted-foreground text-sm'>{t('common.none')}</p>
        )}
        {statistics.vendor_breakdown.map((item) => (
          <article
            className='border-border rounded-lg border p-3 text-sm'
            key={`${item.vendor_key}:${item.vendor_id}`}
          >
            <p className='font-medium'>{item.vendor_key || '-'}</p>
            <code className='text-muted-foreground text-xs'>
              {item.vendor_id}
            </code>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('pricingGroups.breakdown.vendorValues', {
                missing: item.missing,
                total: item.total,
              })}
            </p>
          </article>
        ))}
      </section>
    )
  }
  if (tab === 'group-model-analysis') {
    return (
      <section className='grid content-start gap-2'>
        <h2 className='font-semibold'>{t('pricingGroups.breakdown.group')}</h2>
        {statistics.group_breakdown.length === 0 && (
          <p className='text-muted-foreground text-sm'>{t('common.none')}</p>
        )}
        {statistics.group_breakdown.map((item) => (
          <article
            className='border-border rounded-lg border p-3 text-sm'
            key={item.group_name}
          >
            <p className='font-medium'>{item.group_name}</p>
            <p className='text-muted-foreground text-xs'>
              {t('pricingGroups.breakdown.groupValues', {
                count: item.model_count,
              })}
            </p>
          </article>
        ))}
      </section>
    )
  }
  return (
    <section className='grid content-start gap-2'>
      <h2 className='font-semibold'>
        {t('pricingGroups.breakdown.availability')}
      </h2>
      {statistics.group_catalog_breakdown.length === 0 && (
        <p className='text-muted-foreground text-sm'>{t('common.none')}</p>
      )}
      {statistics.group_catalog_breakdown.map((item) => (
        <article
          className='border-border rounded-lg border p-3 text-sm'
          key={`${item.root_visible}:${item.ratio_available}`}
        >
          <div className='flex flex-wrap gap-2'>
            <VisibilityBadge visible={item.root_visible} />
            <Badge variant={item.ratio_available ? 'success' : 'neutral'}>
              {item.ratio_available
                ? t('pricingGroups.ratio.available')
                : t('pricingGroups.ratio.unavailable')}
            </Badge>
          </div>
          <p className='text-muted-foreground mt-2 text-xs'>
            {t('pricingGroups.breakdown.availabilityValues', {
              count: item.count,
            })}
          </p>
        </article>
      ))}
    </section>
  )
}

function SummaryGrid({
  statistics,
}: {
  statistics?: PricingCatalogStatistics
}) {
  const { t } = useTranslation()
  const items = [
    {
      icon: Database01Icon,
      label: t('pricingGroups.metric.pricing'),
      value: statistics?.total,
    },
    {
      icon: Chart01Icon,
      label: t('pricingGroups.metric.groups'),
      value: statistics?.group_total,
    },
    {
      icon: Alert02Icon,
      label: t('pricingGroups.metric.missing'),
      value: statistics?.missing,
    },
  ] as const
  return (
    <dl className='grid gap-3 sm:grid-cols-3'>
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

export function PricingGroupsPage({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<PricingGroupSearch>) => void
  search: PricingGroupSearch
  siteId?: string
}) {
  const { t } = useTranslation()
  const [initialJob, setInitialJob] = useState<StatisticsExportJobItem>()
  const canonicalizedSearch = useRef(false)
  const validSiteId = siteId == null || isIdString(siteId)
  const currentParams = useMemo(() => params(search), [search])
  const overviewParams = useMemo(() => params(buildPricingGroupSearch({})), [])
  const groupsParams =
    search.tab === 'site-analysis' ? overviewParams : currentParams
  const siteParams = useMemo(
    () => ({
      p: 1,
      page_size: 100,
      sort_by: 'name',
      sort_order: 'asc' as const,
    }),
    []
  )
  const parsedSiteId =
    siteId && isIdString(siteId) ? parseIdString(siteId) : undefined
  useEffect(() => {
    if (isPricingAnalysisTab(search.tab) && hasPricingAnalysisFilters(search)) {
      onSearchChange(changePricingGroupTab(search.tab))
    } else if (search.tab === 'groups' && search.group !== '') {
      onSearchChange({ group: '', page: 1 })
    } else if (!canonicalizedSearch.current) {
      canonicalizedSearch.current = true
      onSearchChange({})
    }
  }, [onSearchChange, search])
  const sitesQuery = useQuery({
    enabled: siteId == null,
    queryFn: () => listSites(siteParams),
    queryKey: siteKeys.list(siteParams),
    staleTime: 5 * 60_000,
  })
  const pricingQuery = useQuery({
    enabled: validSiteId && search.tab === 'pricing',
    placeholderData: keepPreviousData,
    queryFn: () =>
      parsedSiteId
        ? listSitePricingCatalog(parsedSiteId, currentParams)
        : listPricingCatalog(currentParams),
    queryKey: parsedSiteId
      ? pricingGroupKeys.site(siteId ?? '', 'pricing', currentParams)
      : pricingGroupKeys.global('pricing', currentParams),
  })
  const groupsQuery = useQuery({
    enabled:
      validSiteId &&
      (search.tab === 'groups' || search.tab === 'site-analysis'),
    placeholderData: keepPreviousData,
    queryFn: () =>
      parsedSiteId
        ? listSitePricingGroups(parsedSiteId, groupsParams)
        : listPricingGroups(groupsParams),
    queryKey: parsedSiteId
      ? pricingGroupKeys.site(siteId ?? '', 'groups', groupsParams)
      : pricingGroupKeys.global('groups', groupsParams),
  })
  const statisticsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      parsedSiteId
        ? getSitePricingCatalogStatistics(parsedSiteId, overviewParams)
        : getPricingCatalogStatistics(overviewParams),
    queryKey: parsedSiteId
      ? pricingGroupKeys.site(siteId ?? '', 'statistics', overviewParams)
      : pricingGroupKeys.global('statistics', overviewParams),
  })
  const exportMutation = useMutation({
    mutationFn: (format: StatisticsExportFormat) =>
      createStatisticsExport(
        buildPricingGroupExportRequest(format, search, parsedSiteId)
      ),
    onError: (error) =>
      toast.error(t(dynamicI18nKey('api', getApiErrorTranslationKey(error)))),
    onSuccess: (job) => {
      setInitialJob(job)
      onSearchChange({ exportId: job.id })
    },
  })
  const pricingColumns = useMemo<ColumnDef<PricingCatalogItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='grid min-w-48 gap-1'>
            <strong>{row.original.model_name}</strong>
            <span className='text-muted-foreground text-xs'>
              {row.original.site_name} · {row.original.site_id}
            </span>
            <span className='text-muted-foreground text-xs'>
              {row.original.vendor_key || '-'} · {row.original.vendor_id}
            </span>
            <span className='text-muted-foreground text-xs'>
              {row.original.description || '-'}
            </span>
          </div>
        ),
        header: t('pricingGroups.pricing.identity'),
        id: 'identity',
      },
      {
        cell: ({ row }) => (
          <dl className='grid min-w-44 gap-1 text-xs'>
            <div>
              <dt className='inline'>{t('pricingGroups.ratio.model')}：</dt>
              <dd className='inline'>{row.original.model_ratio}</dd>
            </div>
            <div>
              <dt className='inline'>{t('pricingGroups.ratio.price')}：</dt>
              <dd className='inline'>{row.original.model_price}</dd>
            </div>
            <div>
              <dt className='inline'>
                {t('pricingGroups.ratio.completion')}：
              </dt>
              <dd className='inline'>{row.original.completion_ratio}</dd>
            </div>
            <div>
              <dt className='inline'>{t('pricingGroups.ratio.cache')}：</dt>
              <dd className='inline'>
                {formatNumericDisplayValue(row.original.cache_ratio)}
              </dd>
            </div>
          </dl>
        ),
        header: t('pricingGroups.pricing.ratios'),
        id: 'ratios',
      },
      {
        cell: ({ row }) => <TextBadges values={row.original.enable_groups} />,
        header: t('pricingGroups.pricing.groups'),
        id: 'groups',
      },
      {
        cell: ({ row }) => (
          <TextBadges values={row.original.supported_endpoint_types} />
        ),
        header: t('pricingGroups.pricing.endpoints'),
        id: 'endpoints',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-32 gap-1'>
            <VisibilityBadge visible={row.original.root_visible} />
            <StateBadge state={row.original.remote_state} />
            <DataStatusBadge status={row.original.data_status} />
            <span className='text-muted-foreground text-xs'>
              {timestamp(row.original.collected_at)}
            </span>
          </div>
        ),
        header: t('common.status'),
        id: 'status',
      },
    ],
    [t]
  )
  const groupColumns = useMemo<ColumnDef<PricingGroupItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='grid min-w-48 gap-1'>
            <strong>{row.original.name}</strong>
            <span className='text-muted-foreground text-xs'>
              {row.original.site_name} · {row.original.site_id}
            </span>
            <span className='text-muted-foreground text-xs'>
              {row.original.description || '-'}
            </span>
          </div>
        ),
        header: t('pricingGroups.groups.identity'),
        id: 'identity',
      },
      {
        cell: ({ row }) => (
          <code>{formatNumericDisplayValue(row.original.ratio)}</code>
        ),
        header: t('pricingGroups.groups.ratio'),
        id: 'ratio',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-32 gap-1'>
            <VisibilityBadge visible={row.original.root_visible} />
            <StateBadge state={row.original.remote_state} />
            <DataStatusBadge status={row.original.data_status} />
            <span className='text-muted-foreground text-xs'>
              {timestamp(row.original.collected_at)}
            </span>
          </div>
        ),
        header: t('common.status'),
        id: 'status',
      },
    ],
    [t]
  )
  const statistics = statisticsQuery.data
  const listTab = search.tab === 'pricing' || search.tab === 'groups'
  const tabs = [
    {
      count: statistics?.total,
      icon: Database01Icon,
      label: t('pricingGroups.tabs.pricing'),
      value: 'pricing',
    },
    {
      count: statistics?.group_total,
      icon: Chart01Icon,
      label: t('pricingGroups.tabs.groups'),
      value: 'groups',
    },
    {
      icon: Chart01Icon,
      label: t('pricingGroups.tabs.siteAnalysis'),
      value: 'site-analysis',
    },
    {
      icon: Chart01Icon,
      label: t('pricingGroups.tabs.vendorAnalysis'),
      value: 'vendor-analysis',
    },
    {
      icon: Chart01Icon,
      label: t('pricingGroups.tabs.groupModelAnalysis'),
      value: 'group-model-analysis',
    },
    {
      icon: Chart01Icon,
      label: t('pricingGroups.tabs.groupAvailabilityAnalysis'),
      value: 'group-availability-analysis',
    },
  ] as const
  const purpose = {
    'group-availability-analysis': {
      description: t(
        'pricingGroups.purpose.groupAvailabilityAnalysis.description'
      ),
      title: t('pricingGroups.purpose.groupAvailabilityAnalysis.title'),
    },
    'group-model-analysis': {
      description: t('pricingGroups.purpose.groupModelAnalysis.description'),
      title: t('pricingGroups.purpose.groupModelAnalysis.title'),
    },
    groups: {
      description: t('pricingGroups.purpose.groups.description'),
      title: t('pricingGroups.purpose.groups.title'),
    },
    pricing: {
      description: t('pricingGroups.purpose.pricing.description'),
      title: t('pricingGroups.purpose.pricing.title'),
    },
    'site-analysis': {
      description: t('pricingGroups.purpose.siteAnalysis.description'),
      title: t('pricingGroups.purpose.siteAnalysis.title'),
    },
    'vendor-analysis': {
      description: t('pricingGroups.purpose.vendorAnalysis.description'),
      title: t('pricingGroups.purpose.vendorAnalysis.title'),
    },
  }[search.tab]
  let activeDataStatus = statistics?.data_status
  if (search.tab === 'pricing') {
    activeDataStatus = pricingQuery.data?.data_status
  } else if (search.tab === 'groups') {
    activeDataStatus = groupsQuery.data?.data_status
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
                {t('pricingGroups.export', { format: format.toUpperCase() })}
              </Button>
            ))
          : undefined
      }
      description={
        siteId
          ? t('pricingGroups.siteDescription', { id: siteId })
          : t('pricingGroups.description')
      }
      fixedContent
      title={siteId ? t('pricingGroups.siteTitle') : t('pricingGroups.title')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('pricingGroups.backToSite')}
          </DetailBackLink>
        )}
        <SummaryGrid statistics={statistics} />
        <Tabs
          onValueChange={(tab) =>
            onSearchChange(
              changePricingGroupTab(tab as PricingGroupSearch['tab'])
            )
          }
          value={search.tab}
        >
          <TabsList
            aria-label={t('pricingGroups.tabs.label')}
            className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'
          >
            {tabs.map((tab) => (
              <TabsTrigger key={tab.value} value={tab.value}>
                <HugeiconsIcon icon={tab.icon} size={15} strokeWidth={2} />
                {tab.label}
                {'count' in tab && tab.count != null && (
                  <Badge className='px-1.5 font-mono' variant='secondary'>
                    {tab.count}
                  </Badge>
                )}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <section className='border-border bg-muted/30 flex items-start gap-3 rounded-xl border p-4'>
          <span className='bg-background text-muted-foreground ring-foreground/10 flex size-9 shrink-0 items-center justify-center rounded-lg ring-1'>
            <HugeiconsIcon
              icon={search.tab === 'pricing' ? Database01Icon : Chart01Icon}
              size={18}
              strokeWidth={2}
            />
          </span>
          <div className='min-w-0 flex-1'>
            <div className='flex flex-wrap items-center gap-2'>
              <h2 className='font-medium'>{purpose.title}</h2>
              {activeDataStatus && (
                <DataStatusBadge status={activeDataStatus} />
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
            className='justify-self-start'
            onClick={() => void statisticsQuery.refetch()}
            variant='outline'
          >
            {t('common.retry')}
          </Button>
        )}
        {search.tab === 'pricing' && (
          <DataTable
            ariaLabel={t('pricingGroups.pricing.table')}
            columns={pricingColumns}
            data={pricingQuery.data?.items ?? []}
            emptyDescription={t('pricingGroups.emptyDescription')}
            emptyTitle={t('pricingGroups.pricing.empty')}
            error={!validSiteId || pricingQuery.isError}
            fetching={pricingQuery.isFetching}
            loading={pricingQuery.isPending}
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={() => void pricingQuery.refetch()}
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(item) => (
              <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
                <div className='flex items-start justify-between gap-2'>
                  <div>
                    <strong>{item.model_name}</strong>
                    <p className='text-muted-foreground text-xs'>
                      {item.site_name} · {item.site_id}
                    </p>
                  </div>
                  <VisibilityBadge visible={item.root_visible} />
                </div>
                <p className='text-sm'>
                  {item.vendor_key || '-'} · {item.vendor_id}
                </p>
                <code className='text-xs'>
                  {t('pricingGroups.ratio.model')} {item.model_ratio} ·{' '}
                  {t('pricingGroups.ratio.price')} {item.model_price}
                </code>
                <TextBadges values={item.enable_groups} />
                <TextBadges values={item.supported_endpoint_types} />
                <div className='flex flex-wrap gap-2'>
                  <StateBadge state={item.remote_state} />
                  <DataStatusBadge status={item.data_status} />
                </div>
              </article>
            )}
            total={pricingQuery.data?.total ?? 0}
          />
        )}
        {search.tab === 'groups' && (
          <DataTable
            ariaLabel={t('pricingGroups.groups.table')}
            columns={groupColumns}
            data={groupsQuery.data?.items ?? []}
            emptyDescription={t('pricingGroups.emptyDescription')}
            emptyTitle={t('pricingGroups.groups.empty')}
            error={!validSiteId || groupsQuery.isError}
            fetching={groupsQuery.isFetching}
            loading={groupsQuery.isPending}
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={() => void groupsQuery.refetch()}
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(item) => (
              <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
                <div className='flex items-start justify-between gap-2'>
                  <div>
                    <strong>{item.name}</strong>
                    <p className='text-muted-foreground text-xs'>
                      {item.site_name} · {item.site_id}
                    </p>
                  </div>
                  <VisibilityBadge visible={item.root_visible} />
                </div>
                <p>{item.description || '-'}</p>
                <code>{formatNumericDisplayValue(item.ratio)}</code>
                <div className='flex flex-wrap gap-2'>
                  <StateBadge state={item.remote_state} />
                  <DataStatusBadge status={item.data_status} />
                </div>
              </article>
            )}
            total={groupsQuery.data?.total ?? 0}
          />
        )}
        {isPricingAnalysisTab(search.tab) && (
          <div className='min-h-0 flex-1 overflow-y-auto'>
            {search.tab === 'site-analysis' && (
              <div className='grid gap-5'>
                {statistics && (
                  <SiteBreakdown
                    items={statistics.site_breakdown}
                    kind='pricing'
                  />
                )}
                {groupsQuery.data && (
                  <SiteBreakdown
                    items={groupsQuery.data.site_breakdown}
                    kind='groups'
                  />
                )}
              </div>
            )}
            {statistics && search.tab !== 'site-analysis' && (
              <TypedBreakdown statistics={statistics} tab={search.tab} />
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
