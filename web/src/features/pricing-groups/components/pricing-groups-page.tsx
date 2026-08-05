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
import { DebouncedInput } from '@/components/data/debounced-input'
import { FacetedFilter } from '@/components/data/faceted-filter'
import { MetricValue } from '@/components/data/metric-value'
import { MultiFacetedFilter } from '@/components/data/multi-faceted-filter'
import { QueryStateAlert } from '@/components/data/query-state-alert'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { listAllSites } from '@/features/sites/api'
import { siteKeys } from '@/features/sites/query-keys'
import type { SiteListItem } from '@/features/sites/types'
import { createStatisticsExport } from '@/features/statistics/api'
import { ExportTaskSheet } from '@/features/statistics/components/export-task-sheet'
import type {
  StatisticsExportFormat,
  StatisticsExportJobItem,
} from '@/features/statistics/types'
import { useLastValidPage } from '@/hooks/use-last-valid-page'
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
import {
  hiddenPricingValueCount,
  keyedPricingValues,
  visiblePricingText,
  visiblePricingValues,
} from '../presentation'
import { pricingGroupKeys } from '../query-keys'
import {
  buildPricingGroupSearch,
  changePricingGroupTab,
  type PricingGroupSearch,
} from '../search'
import type {
  PricingBillingMode,
  PricingCatalogItem,
  PricingCatalogQueryParams,
  PricingCatalogSiteBreakdown,
  PricingCatalogSiteOverview,
  PricingCatalogState,
  PricingCatalogStatistics,
  PricingGroupItem,
} from '../types'

function params(search: PricingGroupSearch): PricingCatalogQueryParams {
  return {
    billing_mode: search.billingMode,
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

function BooleanBadge({
  value,
  yes,
  no,
}: {
  value: boolean
  yes: string
  no: string
}) {
  return (
    <Badge variant={value ? 'success' : 'neutral'}>{value ? yes : no}</Badge>
  )
}

function BillingModeBadge({ mode }: { mode: string }) {
  const { t } = useTranslation()
  let label = t('pricingGroups.billingMode.token')
  if (mode === 'fixed') {
    label = t('pricingGroups.billingMode.fixed')
  } else if (mode === 'tiered_expr') {
    label = t('pricingGroups.billingMode.tieredExpr')
  }
  return <Badge variant='secondary'>{label}</Badge>
}

function PricingSourceLabel({ source }: { source: string }) {
  const { t } = useTranslation()
  if (source === 'fixed') {
    return t('pricingGroups.pricingSource.fixed')
  }
  if (source === 'tiered_expr') {
    return t('pricingGroups.pricingSource.tiered_expr')
  }
  if (source === 'token_explicit') {
    return t('pricingGroups.pricingSource.token_explicit')
  }
  return t('pricingGroups.pricingSource.token_default')
}

function TextBadges({
  values,
  emptyLabel,
  ariaLabel,
}: {
  values: string[]
  emptyLabel?: string
  ariaLabel?: string
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  if (values.length === 0) {
    return (
      <span className='text-muted-foreground'>
        {emptyLabel ?? t('common.none')}
      </span>
    )
  }
  const visibleValues = visiblePricingValues(values, expanded)
  const hiddenCount = hiddenPricingValueCount(values, expanded)
  return (
    <div
      aria-label={ariaLabel}
      className='grid max-w-80 min-w-0 gap-1.5'
      role={ariaLabel ? 'group' : undefined}
    >
      <div className='flex min-w-0 flex-wrap gap-1'>
        {keyedPricingValues(visibleValues).map(({ key, value }) => (
          <Badge
            className='max-w-full text-left break-all whitespace-normal'
            key={key}
            variant='neutral'
          >
            {value}
          </Badge>
        ))}
      </div>
      {(hiddenCount > 0 || expanded) && (
        <Button
          aria-expanded={expanded}
          className='h-auto w-fit px-0 py-0 text-xs'
          onClick={() => setExpanded((value) => !value)}
          type='button'
          variant='link'
        >
          {expanded
            ? t('common.collapse')
            : `${t('common.expand')} (+${hiddenCount})`}
        </Button>
      )}
    </div>
  )
}

function MappingList({
  values,
  emptyLabel,
}: {
  values: Record<string, string>
  emptyLabel: string
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const entries = Object.entries(values).sort(([left], [right]) =>
    left.localeCompare(right)
  )
  if (entries.length === 0) {
    return <span className='text-muted-foreground'>{emptyLabel}</span>
  }
  const visibleEntries = visiblePricingValues(entries, expanded)
  const hiddenCount = hiddenPricingValueCount(entries, expanded)
  return (
    <div className='grid min-w-0 gap-1.5'>
      {visibleEntries.map(([name, value]) => (
        <div
          className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-start gap-1 text-xs'
          key={name}
        >
          <Badge
            className='min-w-0 break-all whitespace-normal'
            variant='neutral'
          >
            {name}
          </Badge>
          <span className='text-muted-foreground'>→</span>
          <code className='min-w-0 break-all whitespace-normal'>{value}</code>
        </div>
      ))}
      {(hiddenCount > 0 || expanded) && (
        <Button
          aria-expanded={expanded}
          className='h-auto w-fit px-0 py-0 text-xs'
          onClick={() => setExpanded((value) => !value)}
          type='button'
          variant='link'
        >
          {expanded
            ? t('common.collapse')
            : `${t('common.expand')} (+${hiddenCount})`}
        </Button>
      )}
    </div>
  )
}

function ExpandableAuditText({ value }: { value: string }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const canExpand = visiblePricingText(value, false) !== value
  return (
    <div className='grid min-w-0 gap-1'>
      <code className='min-w-0 break-all whitespace-normal'>
        {visiblePricingText(value || '-', expanded)}
      </code>
      {canExpand && (
        <Button
          aria-expanded={expanded}
          className='h-auto w-fit px-0 py-0 text-xs'
          onClick={() => setExpanded((current) => !current)}
          type='button'
          variant='link'
        >
          {expanded ? t('common.collapse') : t('common.expand')}
        </Button>
      )}
    </div>
  )
}

function PricingAuditMetadata({ item }: { item: PricingCatalogItem }) {
  const { t } = useTranslation()
  const values = [
    [t('pricingGroups.audit.vendorId'), item.vendor_id],
    [t('pricingGroups.audit.quotaType'), item.quota_type],
    [t('pricingGroups.audit.ownerBy'), item.owner_by || '-'],
    [t('pricingGroups.audit.pricingVersion'), item.pricing_version || '-'],
  ] as const
  return (
    <div className='grid max-w-80 min-w-64 gap-2 text-xs'>
      <dl className='grid grid-cols-[auto_minmax(0,1fr)] gap-x-2 gap-y-1'>
        {values.map(([label, value]) => (
          <div className='contents' key={label}>
            <dt className='text-muted-foreground'>{label}</dt>
            <dd className='min-w-0 break-all'>{value}</dd>
          </div>
        ))}
      </dl>
      <div>
        <p className='text-muted-foreground mb-1'>
          {t('pricingGroups.audit.tags')}
        </p>
        <ExpandableAuditText value={item.tags} />
      </div>
      <div>
        <p className='text-muted-foreground mb-1'>
          {t('pricingGroups.audit.icon')}
        </p>
        <ExpandableAuditText value={item.icon} />
      </div>
    </div>
  )
}

function completeSiteCount(
  sites: readonly PricingCatalogSiteBreakdown[],
  statisticsSites: readonly PricingCatalogSiteOverview[],
  tab: PricingGroupSearch['tab']
) {
  if (sites.length > 0) {
    return sites.filter((site) => site.data_status === 'complete').length
  }
  return statisticsSites.filter((site) =>
    tab === 'pricing'
      ? site.pricing_data_status === 'complete'
      : site.group_data_status === 'complete'
  ).length
}

function CompletenessSummary({
  asOf,
  pageSites,
  statisticsSites,
  tab,
}: {
  asOf: number | null | undefined
  pageSites: PricingCatalogSiteBreakdown[]
  statisticsSites: PricingCatalogSiteOverview[]
  tab: PricingGroupSearch['tab']
}) {
  const { t } = useTranslation()
  const complete = completeSiteCount(pageSites, statisticsSites, tab)
  const expected = pageSites.length || statisticsSites.length
  return (
    <section
      aria-label={t('pricingGroups.completeness.title')}
      className='border-border bg-background grid min-w-0 gap-2 rounded-lg border px-3 py-2 text-xs'
    >
      <div className='flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1'>
        <span>
          <span className='text-muted-foreground'>
            {t('pricingGroups.completeness.asOf')}：
          </span>
          {timestamp(asOf ?? null)}
        </span>
        <span>
          <span className='text-muted-foreground'>
            {t('pricingGroups.completeness.completeSites')}：
          </span>
          {complete}/{expected}
        </span>
        <span>
          <span className='text-muted-foreground'>
            {t('pricingGroups.completeness.statisticsSites')}：
          </span>
          {statisticsSites.length}
        </span>
      </div>
      {(pageSites.length > 0 || statisticsSites.length > 0) && (
        <details className='group min-w-0'>
          <summary className='text-primary w-fit cursor-pointer font-medium'>
            {t('pricingGroups.completeness.details')}
          </summary>
          <div className='mt-2 grid min-w-0 gap-2 lg:grid-cols-2'>
            {pageSites.map((site) => (
              <article
                className='border-border bg-muted/30 grid min-w-0 gap-1 rounded-md border p-2'
                key={`page-${site.site_id}`}
              >
                <div className='flex min-w-0 flex-wrap items-center justify-between gap-1'>
                  <strong className='min-w-0 break-words'>
                    {site.site_name} · {site.site_id}
                  </strong>
                  <DataStatusBadge status={site.data_status} />
                </div>
                <span>
                  {t('pricingGroups.completeness.totalMissing', {
                    missing: site.missing,
                    total: site.total,
                  })}
                </span>
                <span className='text-muted-foreground'>
                  {timestamp(site.as_of)}
                </span>
              </article>
            ))}
            {statisticsSites.map((site) => (
              <article
                className='border-border grid min-w-0 gap-1 rounded-md border p-2'
                key={`statistics-${site.site_id}`}
              >
                <strong className='min-w-0 break-words'>
                  {site.site_name} · {site.site_id}
                </strong>
                <div className='flex min-w-0 flex-wrap items-center gap-1'>
                  <DataStatusBadge status={site.pricing_data_status} />
                  <span>
                    {t('pricingGroups.completeness.pricingCounts', {
                      active: site.pricing_active,
                      missing: site.pricing_missing,
                    })}
                  </span>
                  <span className='text-muted-foreground'>
                    {timestamp(site.pricing_as_of)}
                  </span>
                </div>
                <div className='flex min-w-0 flex-wrap items-center gap-1'>
                  <DataStatusBadge status={site.group_data_status} />
                  <span>
                    {t('pricingGroups.completeness.groupCounts', {
                      active: site.group_active,
                      missing: site.group_missing,
                    })}
                  </span>
                  <span className='text-muted-foreground'>
                    {timestamp(site.group_as_of)}
                  </span>
                </div>
              </article>
            ))}
          </div>
        </details>
      )}
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
  onChange: (changes: Partial<PricingGroupSearch>) => void
  search: PricingGroupSearch
  sites: SiteListItem[]
}) {
  const { t } = useTranslation()
  const reset = buildPricingGroupSearch({
    pageSize: search.pageSize,
    tab: search.tab,
  })
  const hasActiveFilters = hasFilterChanges(search, reset, [
    'billingMode',
    'group',
    'keyword',
    'siteIds',
    'states',
  ])
  const billingModes: { label: string; value: PricingBillingMode }[] = [
    { label: t('pricingGroups.billingMode.token'), value: 'token' },
    { label: t('pricingGroups.billingMode.fixed'), value: 'fixed' },
    { label: t('pricingGroups.billingMode.tieredExpr'), value: 'tiered_expr' },
  ]
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
        <DebouncedInput
          aria-label={t('pricingGroups.filters.keyword')}
          className='h-10 pl-8 sm:h-8'
          onValueChange={(keyword) => onChange({ keyword, page: 1 })}
          placeholder={
            search.tab === 'pricing'
              ? t('pricingGroups.filters.modelPlaceholder')
              : t('pricingGroups.filters.groupPlaceholder')
          }
          value={search.keyword}
        />
      </label>
      {search.tab === 'pricing' && (
        <>
          <Input
            aria-label={t('pricingGroups.filters.group')}
            className='h-10 w-36 sm:h-8'
            onChange={(event) =>
              onChange({ group: event.target.value, page: 1 })
            }
            placeholder={t('pricingGroups.filters.group')}
            value={search.group}
          />
          <FacetedFilter
            clearLabel={t('pricingGroups.filters.allBillingModes')}
            onChange={(value) =>
              onChange({
                billingMode: billingModes.some((item) => item.value === value)
                  ? (value as PricingBillingMode)
                  : undefined,
                page: 1,
              })
            }
            options={billingModes}
            title={t('pricingGroups.filters.billingMode')}
            value={search.billingMode ?? ''}
          />
        </>
      )}
      {global && (
        <MultiFacetedFilter
          clearLabel={t('pricingGroups.filters.allSites')}
          onChange={(values) =>
            onChange({
              page: 1,
              siteIds: values.filter(isIdString).map(parseIdString),
            })
          }
          options={sites.map((site) => ({ label: site.name, value: site.id }))}
          title={t('pricingGroups.filters.site')}
          values={search.siteIds}
        />
      )}
      <FacetedFilter
        clearLabel={t('pricingGroups.filters.allStates')}
        onChange={(value) =>
          onChange({
            page: 1,
            states: value === 'normal' || value === 'missing' ? [value] : [],
          })
        }
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

function SummaryGrid({
  statistics,
}: {
  statistics?: PricingCatalogStatistics
}) {
  const { t } = useTranslation()
  const items = [
    {
      icon: Database01Icon,
      label: t('pricingGroups.metric.sites'),
      value: statistics?.site_count,
    },
    {
      icon: Database01Icon,
      label: t('pricingGroups.metric.pricingActive'),
      value: statistics?.pricing_active,
    },
    {
      icon: Chart01Icon,
      label: t('pricingGroups.metric.groupActive'),
      value: statistics?.group_active,
    },
    {
      icon: Alert02Icon,
      label: t('pricingGroups.metric.pricingMissing'),
      value: statistics?.pricing_missing,
    },
  ] as const
  return (
    <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
      {items.map(({ icon, label, value }) => (
        <div
          className='bg-card text-card-foreground ring-foreground/10 flex items-center gap-3 rounded-xl p-4 ring-1'
          key={label}
        >
          <span className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg'>
            <HugeiconsIcon icon={icon} size={18} strokeWidth={2} />
          </span>
          <dl>
            <dt className='text-muted-foreground text-xs'>{label}</dt>
            <dd className='mt-0.5 text-2xl font-semibold tracking-tight'>
              {value == null ? '-' : <MetricValue value={value} />}
            </dd>
          </dl>
        </div>
      ))}
    </div>
  )
}

function PriceDimensions({ item }: { item: PricingCatalogItem }) {
  const { t } = useTranslation()
  const values = [
    [t('pricingGroups.ratio.model'), item.model_ratio],
    [t('pricingGroups.ratio.price'), item.model_price],
    [t('pricingGroups.ratio.completion'), item.completion_ratio],
    [t('pricingGroups.ratio.cache'), item.cache_ratio],
    [t('pricingGroups.ratio.createCache'), item.create_cache_ratio],
    [t('pricingGroups.ratio.image'), item.image_ratio],
    [t('pricingGroups.ratio.audio'), item.audio_ratio],
    [t('pricingGroups.ratio.audioCompletion'), item.audio_completion_ratio],
  ] as const
  return (
    <div className='grid min-w-52 gap-1 text-xs'>
      {values.map(([label, value]) => (
        <div className='flex justify-between gap-4' key={label}>
          <span className='text-muted-foreground'>{label}</span>
          <code>{formatNumericDisplayValue(value)}</code>
        </div>
      ))}
    </div>
  )
}

export function PricingGroupsPage({
  onPageReplace,
  onSearchChange,
  search,
  siteId,
}: {
  onPageReplace: (page: number) => void
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
  const siteParams = useMemo(
    () => ({
      sort_by: 'name',
      sort_order: 'asc' as const,
    }),
    []
  )
  const parsedSiteId =
    siteId && isIdString(siteId) ? parseIdString(siteId) : undefined

  useEffect(() => {
    if (
      search.tab === 'groups' &&
      (search.group !== '' || search.billingMode != null)
    ) {
      onSearchChange({ billingMode: undefined, group: '', page: 1 })
    } else if (!canonicalizedSearch.current) {
      canonicalizedSearch.current = true
      onSearchChange({})
    }
  }, [onSearchChange, search])

  const sitesQuery = useQuery({
    enabled: siteId == null,
    queryFn: () => listAllSites(siteParams),
    queryKey: siteKeys.options(siteParams),
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
    enabled: validSiteId && search.tab === 'groups',
    placeholderData: keepPreviousData,
    queryFn: () =>
      parsedSiteId
        ? listSitePricingGroups(parsedSiteId, currentParams)
        : listPricingGroups(currentParams),
    queryKey: parsedSiteId
      ? pricingGroupKeys.site(siteId ?? '', 'groups', currentParams)
      : pricingGroupKeys.global('groups', currentParams),
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
  const activePageQuery = search.tab === 'pricing' ? pricingQuery : groupsQuery
  useLastValidPage({
    isFetching: activePageQuery.isFetching,
    isPlaceholderData: activePageQuery.isPlaceholderData,
    onReplace: onPageReplace,
    page: search.page,
    pageSize: activePageQuery.data?.page_size ?? search.pageSize,
    total: activePageQuery.data?.total,
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
              {t('pricingGroups.pricing.vendorName')}：
              {row.original.vendor_name || '-'}
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
          <div className='grid min-w-40 gap-2 text-xs'>
            <BillingModeBadge mode={row.original.billing_mode} />
            <span>
              {t('pricingGroups.pricing.source')}：
              <PricingSourceLabel source={row.original.pricing_source} />
            </span>
            <BooleanBadge
              value={row.original.ability_available}
              yes={t('pricingGroups.pricing.abilityAvailable')}
              no={t('pricingGroups.pricing.abilityUnavailable')}
            />
          </div>
        ),
        header: t('pricingGroups.pricing.billing'),
        id: 'billing',
      },
      {
        cell: ({ row }) => <PricingAuditMetadata item={row.original} />,
        header: t('pricingGroups.audit.title'),
        id: 'audit',
      },
      {
        cell: ({ row }) => <PriceDimensions item={row.original} />,
        header: t('pricingGroups.pricing.ratios'),
        id: 'ratios',
      },
      {
        cell: ({ row }) =>
          row.original.billing_expr ? (
            <pre className='bg-muted max-h-28 max-w-96 min-w-56 overflow-auto rounded-md p-2 text-xs whitespace-pre-wrap'>
              {row.original.billing_expr}
            </pre>
          ) : (
            <span className='text-muted-foreground'>
              {t('pricingGroups.pricing.noExpression')}
            </span>
          ),
        header: t('pricingGroups.pricing.expression'),
        id: 'expression',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-44 gap-3'>
            <div>
              <p className='text-muted-foreground mb-1 text-xs'>
                {t('pricingGroups.pricing.groups')}
              </p>
              <TextBadges
                ariaLabel={t('pricingGroups.pricing.groups')}
                values={row.original.enable_groups}
              />
            </div>
            <div>
              <p className='text-muted-foreground mb-1 text-xs'>
                {t('pricingGroups.pricing.endpoints')}
              </p>
              <TextBadges
                ariaLabel={t('pricingGroups.pricing.endpoints')}
                values={row.original.supported_endpoint_types}
              />
            </div>
          </div>
        ),
        header: t('pricingGroups.pricing.availability'),
        id: 'availability',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-32 gap-1'>
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
          <div className='grid min-w-48 gap-2'>
            <div>
              <strong>{row.original.name}</strong>
              <p className='text-muted-foreground text-xs'>
                {row.original.site_name} · {row.original.site_id}
              </p>
            </div>
            <BooleanBadge
              value={row.original.user_selectable}
              yes={t('pricingGroups.groups.userSelectable')}
              no={t('pricingGroups.groups.userNotSelectable')}
            />
            <span className='text-muted-foreground text-xs'>
              {row.original.description ||
                t('pricingGroups.groups.noDescription')}
            </span>
          </div>
        ),
        header: t('pricingGroups.groups.identity'),
        id: 'identity',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-40 gap-2 text-xs'>
            <div className='flex justify-between gap-3'>
              <span className='text-muted-foreground'>
                {t('pricingGroups.groups.ratio')}
              </span>
              <code>{formatNumericDisplayValue(row.original.ratio)}</code>
            </div>
            <div className='flex justify-between gap-3'>
              <span className='text-muted-foreground'>
                {t('pricingGroups.groups.topupRatio')}
              </span>
              <code>{formatNumericDisplayValue(row.original.topup_ratio)}</code>
            </div>
            <div>
              {t('pricingGroups.groups.autoPriority')}：
              {row.original.auto_priority ??
                t('pricingGroups.groups.notAutoGroup')}
            </div>
            <BooleanBadge
              value={row.original.default_use_auto_group}
              yes={t('pricingGroups.groups.defaultAutoEnabled')}
              no={t('pricingGroups.groups.defaultAutoDisabled')}
            />
          </div>
        ),
        header: t('pricingGroups.groups.billing'),
        id: 'billing',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-72 gap-3'>
            <div>
              <p className='text-muted-foreground mb-1 text-xs'>
                {t('pricingGroups.groups.activePricing', {
                  count: row.original.active_pricing_count,
                })}
              </p>
              <TextBadges
                emptyLabel={t('pricingGroups.groups.noModels')}
                values={row.original.model_names}
              />
            </div>
            <div>
              <p className='text-muted-foreground mb-1 text-xs'>
                {t('pricingGroups.groups.missingPricing', {
                  count: row.original.missing_pricing_count,
                })}
              </p>
              <TextBadges
                emptyLabel={t('pricingGroups.groups.noMissingModels')}
                values={row.original.missing_model_names}
              />
            </div>
          </div>
        ),
        header: t('pricingGroups.groups.models'),
        id: 'models',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-64 gap-3'>
            <div>
              <p className='text-muted-foreground mb-1 text-xs'>
                {t('pricingGroups.groups.outgoingOverrides')}
              </p>
              <MappingList
                emptyLabel={t('pricingGroups.groups.noOverrides')}
                values={row.original.outgoing_overrides}
              />
            </div>
            <div>
              <p className='text-muted-foreground mb-1 text-xs'>
                {t('pricingGroups.groups.incomingOverrides')}
              </p>
              <MappingList
                emptyLabel={t('pricingGroups.groups.noOverrides')}
                values={row.original.incoming_overrides}
              />
            </div>
          </div>
        ),
        header: t('pricingGroups.groups.overrides'),
        id: 'overrides',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-64 gap-3'>
            <div>
              <p className='text-muted-foreground mb-1 text-xs'>
                {t('pricingGroups.groups.visibleTo')}
              </p>
              <MappingList
                emptyLabel={t('pricingGroups.groups.noVisibilityRules')}
                values={row.original.visible_to_groups}
              />
            </div>
            <div>
              <p className='text-muted-foreground mb-1 text-xs'>
                {t('pricingGroups.groups.hiddenFrom')}
              </p>
              <TextBadges
                emptyLabel={t('pricingGroups.groups.noVisibilityRules')}
                values={row.original.hidden_from_groups}
              />
            </div>
          </div>
        ),
        header: t('pricingGroups.groups.visibilityRules'),
        id: 'visibility',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-32 gap-1'>
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
      {
        cell: ({ row }) => (
          <Button
            onClick={() =>
              onSearchChange({
                billingMode: undefined,
                group: row.original.name,
                keyword: '',
                page: 1,
                siteIds: siteId ? [] : [row.original.site_id],
                states: [],
                tab: 'pricing',
              })
            }
            size='sm'
            variant='outline'
          >
            {t('pricingGroups.inspectPricing')}
          </Button>
        ),
        header: t('common.actions'),
        id: 'actions',
      },
    ],
    [onSearchChange, siteId, t]
  )

  const statistics = statisticsQuery.data
  const tabs = [
    {
      count: statistics?.group_active,
      icon: Chart01Icon,
      label: t('pricingGroups.tabs.groups'),
      value: 'groups',
    },
    {
      count: statistics?.pricing_active,
      icon: Database01Icon,
      label: t('pricingGroups.tabs.pricing'),
      value: 'pricing',
    },
  ] as const
  const purpose =
    search.tab === 'groups'
      ? {
          description: t('pricingGroups.purpose.groups.description'),
          title: t('pricingGroups.purpose.groups.title'),
        }
      : {
          description: t('pricingGroups.purpose.pricing.description'),
          title: t('pricingGroups.purpose.pricing.title'),
        }
  const activeDataStatus =
    search.tab === 'groups'
      ? groupsQuery.data?.data_status
      : pricingQuery.data?.data_status

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
          {t('pricingGroups.export', { format: format.toUpperCase() })}
        </Button>
      ))}
      description={
        siteId
          ? t('pricingGroups.siteDescription', { id: siteId })
          : t('pricingGroups.description')
      }
      fixedContent
      mobileScrollableContent
      title={siteId ? t('pricingGroups.siteTitle') : t('pricingGroups.title')}
    >
      <div className='flex min-w-0 flex-col gap-4 lg:h-full lg:min-h-0'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('pricingGroups.backToSite')}
          </DetailBackLink>
        )}
        <SummaryGrid statistics={statistics} />
        {statisticsQuery.isError && statistics && (
          <QueryStateAlert
            message={t('common.retainedDataRefreshFailed')}
            onRetry={() => void statisticsQuery.refetch()}
          />
        )}
        {statisticsQuery.isError && !statistics && validSiteId && (
          <QueryStateAlert
            message={t('common.dataLoadFailed')}
            onRetry={() => void statisticsQuery.refetch()}
            tone='destructive'
          />
        )}
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
                {tab.count != null && (
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
        <Filters
          global={!siteId}
          onChange={onSearchChange}
          search={search}
          sites={sitesQuery.data?.items ?? []}
        />
        <CompletenessSummary
          asOf={activePageQuery.data?.as_of}
          pageSites={activePageQuery.data?.site_breakdown ?? []}
          statisticsSites={statistics?.sites ?? []}
          tab={search.tab}
        />
        {!siteId && sitesQuery.isError && (
          <QueryStateAlert
            message={t('common.siteOptionsRefreshFailed')}
            onRetry={() => void sitesQuery.refetch()}
          />
        )}
        {activePageQuery.isError && activePageQuery.data && (
          <QueryStateAlert
            message={t('common.retainedDataRefreshFailed')}
            onRetry={() => void activePageQuery.refetch()}
          />
        )}
        {search.tab === 'pricing' && (
          <DataTable
            ariaLabel={t('pricingGroups.pricing.table')}
            columns={pricingColumns}
            data={pricingQuery.data?.items ?? []}
            emptyDescription={t('pricingGroups.emptyDescription')}
            emptyTitle={t('pricingGroups.pricing.empty')}
            error={!validSiteId || (pricingQuery.isError && !pricingQuery.data)}
            fetching={pricingQuery.isFetching}
            loading={pricingQuery.isPending}
            mobileCardBreakpoint='wide'
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={
              validSiteId ? () => void pricingQuery.refetch() : undefined
            }
            page={search.page}
            pageSize={search.pageSize}
            total={pricingQuery.data?.total ?? 0}
            renderMobileCard={(item) => (
              <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
                <div className='min-w-0'>
                  <strong className='break-all'>{item.model_name}</strong>
                  <p className='text-muted-foreground text-xs break-words'>
                    {item.site_name} · {item.site_id}
                  </p>
                  <p className='text-muted-foreground text-xs break-words'>
                    {t('pricingGroups.pricing.vendorName')}：
                    {item.vendor_name || '-'}
                  </p>
                </div>
                <div className='flex flex-wrap gap-2'>
                  <BillingModeBadge mode={item.billing_mode} />
                  <BooleanBadge
                    value={item.ability_available}
                    yes={t('pricingGroups.pricing.abilityAvailable')}
                    no={t('pricingGroups.pricing.abilityUnavailable')}
                  />
                </div>
                <PriceDimensions item={item} />
                {item.billing_expr && (
                  <pre className='bg-muted overflow-auto rounded-md p-2 text-xs break-all whitespace-pre-wrap'>
                    {item.billing_expr}
                  </pre>
                )}
                <PricingAuditMetadata item={item} />
                <div>
                  <p className='text-muted-foreground mb-1 text-xs'>
                    {t('pricingGroups.pricing.groups')}
                  </p>
                  <TextBadges
                    ariaLabel={t('pricingGroups.pricing.groups')}
                    values={item.enable_groups}
                  />
                </div>
                <div>
                  <p className='text-muted-foreground mb-1 text-xs'>
                    {t('pricingGroups.pricing.endpoints')}
                  </p>
                  <TextBadges
                    ariaLabel={t('pricingGroups.pricing.endpoints')}
                    values={item.supported_endpoint_types}
                  />
                </div>
                <div className='flex flex-wrap gap-2'>
                  <StateBadge state={item.remote_state} />
                  <DataStatusBadge status={item.data_status} />
                </div>
              </article>
            )}
            rowHeaderColumnId='identity'
          />
        )}
        {search.tab === 'groups' && (
          <DataTable
            ariaLabel={t('pricingGroups.groups.table')}
            columns={groupColumns}
            data={groupsQuery.data?.items ?? []}
            emptyDescription={t('pricingGroups.emptyDescription')}
            emptyTitle={t('pricingGroups.groups.empty')}
            error={!validSiteId || (groupsQuery.isError && !groupsQuery.data)}
            fetching={groupsQuery.isFetching}
            loading={groupsQuery.isPending}
            mobileCardBreakpoint='wide'
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={validSiteId ? () => void groupsQuery.refetch() : undefined}
            page={search.page}
            pageSize={search.pageSize}
            total={groupsQuery.data?.total ?? 0}
            renderMobileCard={(item) => (
              <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
                <div className='min-w-0'>
                  <strong className='break-words'>{item.name}</strong>
                  <p className='text-muted-foreground text-xs break-words'>
                    {item.site_name} · {item.site_id}
                  </p>
                </div>
                <BooleanBadge
                  value={item.user_selectable}
                  yes={t('pricingGroups.groups.userSelectable')}
                  no={t('pricingGroups.groups.userNotSelectable')}
                />
                <p className='break-words'>
                  {item.description || t('pricingGroups.groups.noDescription')}
                </p>
                <div className='flex gap-4 text-xs'>
                  <span>
                    {t('pricingGroups.groups.ratio')}：
                    <code>{formatNumericDisplayValue(item.ratio)}</code>
                  </span>
                  <span>
                    {t('pricingGroups.groups.topupRatio')}：
                    <code>{formatNumericDisplayValue(item.topup_ratio)}</code>
                  </span>
                </div>
                <div className='text-xs'>
                  {t('pricingGroups.groups.autoPriority')}：
                  {item.auto_priority ?? t('pricingGroups.groups.notAutoGroup')}
                </div>
                <BooleanBadge
                  value={item.default_use_auto_group}
                  yes={t('pricingGroups.groups.defaultAutoEnabled')}
                  no={t('pricingGroups.groups.defaultAutoDisabled')}
                />
                <div className='grid gap-3'>
                  <div>
                    <p className='text-muted-foreground mb-1 text-xs'>
                      {t('pricingGroups.groups.activePricing', {
                        count: item.active_pricing_count,
                      })}
                    </p>
                    <TextBadges
                      emptyLabel={t('pricingGroups.groups.noModels')}
                      values={item.model_names}
                    />
                  </div>
                  <div>
                    <p className='text-muted-foreground mb-1 text-xs'>
                      {t('pricingGroups.groups.missingPricing', {
                        count: item.missing_pricing_count,
                      })}
                    </p>
                    <TextBadges
                      emptyLabel={t('pricingGroups.groups.noMissingModels')}
                      values={item.missing_model_names}
                    />
                  </div>
                  <div>
                    <p className='text-muted-foreground mb-1 text-xs'>
                      {t('pricingGroups.groups.outgoingOverrides')}
                    </p>
                    <MappingList
                      emptyLabel={t('pricingGroups.groups.noOverrides')}
                      values={item.outgoing_overrides}
                    />
                  </div>
                  <div>
                    <p className='text-muted-foreground mb-1 text-xs'>
                      {t('pricingGroups.groups.incomingOverrides')}
                    </p>
                    <MappingList
                      emptyLabel={t('pricingGroups.groups.noOverrides')}
                      values={item.incoming_overrides}
                    />
                  </div>
                  <div>
                    <p className='text-muted-foreground mb-1 text-xs'>
                      {t('pricingGroups.groups.visibleTo')}
                    </p>
                    <MappingList
                      emptyLabel={t('pricingGroups.groups.noVisibilityRules')}
                      values={item.visible_to_groups}
                    />
                  </div>
                  <div>
                    <p className='text-muted-foreground mb-1 text-xs'>
                      {t('pricingGroups.groups.hiddenFrom')}
                    </p>
                    <TextBadges
                      emptyLabel={t('pricingGroups.groups.noVisibilityRules')}
                      values={item.hidden_from_groups}
                    />
                  </div>
                </div>
                <div className='flex flex-wrap gap-2'>
                  <StateBadge state={item.remote_state} />
                  <DataStatusBadge status={item.data_status} />
                </div>
                <span className='text-muted-foreground text-xs'>
                  {timestamp(item.collected_at)}
                </span>
                <Button
                  onClick={() =>
                    onSearchChange({
                      billingMode: undefined,
                      group: item.name,
                      keyword: '',
                      page: 1,
                      siteIds: siteId ? [] : [item.site_id],
                      states: [],
                      tab: 'pricing',
                    })
                  }
                  variant='outline'
                >
                  {t('pricingGroups.inspectPricing')}
                </Button>
              </article>
            )}
            rowHeaderColumnId='identity'
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
