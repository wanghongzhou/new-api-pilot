import { ArrowLeft01Icon, FileExportIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataStatusBadge } from '@/components/data/data-status'
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
  getPricingCatalogStatistics,
  getSitePricingCatalogStatistics,
  listPricingCatalog,
  listPricingGroups,
  listSitePricingCatalog,
  listSitePricingGroups,
} from '../api'
import { buildPricingGroupExportRequest } from '../export-request'
import { pricingGroupKeys } from '../query-keys'
import type { PricingGroupSearch } from '../search'
import type {
  PricingCatalogItem,
  PricingCatalogQueryParams,
  PricingCatalogSiteBreakdown,
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
  if (values.length === 0)
    return <span className='text-muted-foreground'>{t('common.none')}</span>
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
}: {
  global: boolean
  onChange: (changes: Partial<PricingGroupSearch>) => void
  search: PricingGroupSearch
}) {
  const { t } = useTranslation()
  const toggleState = (state: PricingCatalogState) =>
    onChange({
      page: 1,
      states: search.states.includes(state)
        ? search.states.filter((value) => value !== state)
        : [...search.states, state],
    })
  return (
    <section
      aria-labelledby='pricing-group-filters-title'
      className='border-border bg-card grid gap-4 rounded-lg border p-4'
    >
      <div>
        <h2 className='font-medium' id='pricing-group-filters-title'>
          {t('pricingGroups.filters.title')}
        </h2>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t('pricingGroups.filters.description')}
        </p>
      </div>
      <div className='grid gap-3 md:grid-cols-3'>
        <label className='grid gap-1 text-sm'>
          <span>{t('pricingGroups.filters.keyword')}</span>
          <Input
            onChange={(event) =>
              onChange({ keyword: event.target.value, page: 1 })
            }
            value={search.keyword}
          />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('pricingGroups.filters.group')}</span>
          <Input
            onChange={(event) =>
              onChange({ group: event.target.value, page: 1 })
            }
            value={search.group}
          />
        </label>
        {global && (
          <label className='grid gap-1 text-sm'>
            <span>{t('pricingGroups.filters.siteIds')}</span>
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
      <fieldset className='grid gap-1'>
        <legend className='text-sm'>{t('pricingGroups.filters.states')}</legend>
        <div className='flex flex-wrap gap-2'>
          {(['normal', 'missing'] as const).map((state) => (
            <Button
              aria-pressed={search.states.includes(state)}
              key={state}
              onClick={() => toggleState(state)}
              size='sm'
              type='button'
              variant={search.states.includes(state) ? 'secondary' : 'outline'}
            >
              {state === 'normal'
                ? t('pricingGroups.state.normal')
                : t('pricingGroups.state.missing')}
            </Button>
          ))}
        </div>
      </fieldset>
    </section>
  )
}

function SiteBreakdown({ items }: { items: PricingCatalogSiteBreakdown[] }) {
  const { t } = useTranslation()
  return (
    <section className='grid gap-3'>
      <h2 className='font-semibold'>{t('pricingGroups.breakdown.site')}</h2>
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
              {t('pricingGroups.breakdown.values', {
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
    </section>
  )
}

function TypedBreakdowns({
  statistics,
}: {
  statistics: Awaited<ReturnType<typeof getPricingCatalogStatistics>>
}) {
  const { t } = useTranslation()
  return (
    <div className='grid gap-5 lg:grid-cols-3'>
      <section className='grid content-start gap-2'>
        <h2 className='font-semibold'>{t('pricingGroups.breakdown.vendor')}</h2>
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
      <section className='grid content-start gap-2'>
        <h2 className='font-semibold'>{t('pricingGroups.breakdown.group')}</h2>
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
      <section className='grid content-start gap-2'>
        <h2 className='font-semibold'>
          {t('pricingGroups.breakdown.availability')}
        </h2>
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
    </div>
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
  const validSiteId = siteId == null || isIdString(siteId)
  const currentParams = useMemo(() => params(search), [search])
  const parsedSiteId =
    siteId && isIdString(siteId) ? parseIdString(siteId) : undefined
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
        ? getSitePricingCatalogStatistics(parsedSiteId, currentParams)
        : getPricingCatalogStatistics(currentParams),
    queryKey: parsedSiteId
      ? pricingGroupKeys.site(siteId ?? '', 'statistics', currentParams)
      : pricingGroupKeys.global('statistics', currentParams),
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
              <dd className='inline'>{row.original.cache_ratio ?? '-'}</dd>
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
        cell: ({ row }) => <code>{row.original.ratio ?? '-'}</code>,
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
      title={siteId ? t('pricingGroups.siteTitle') : t('pricingGroups.title')}
    >
      <div className='grid min-w-0 gap-6'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('pricingGroups.backToSite')}
          </DetailBackLink>
        )}
        <section
          className='border-primary/30 bg-primary/5 rounded-lg border p-4'
          role='note'
        >
          <p className='font-medium'>{t('pricingGroups.boundary.title')}</p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('pricingGroups.boundary.description')}
          </p>
        </section>
        <div
          className='flex flex-wrap gap-2'
          role='tablist'
          aria-label={t('pricingGroups.tabs.label')}
        >
          {(['pricing', 'groups'] as const).map((tab) => (
            <Button
              aria-selected={search.tab === tab}
              key={tab}
              onClick={() => onSearchChange({ page: 1, tab })}
              role='tab'
              variant={search.tab === tab ? 'secondary' : 'outline'}
            >
              {tab === 'pricing'
                ? t('pricingGroups.tabs.pricing')
                : t('pricingGroups.tabs.groups')}
            </Button>
          ))}
        </div>
        <Filters global={!siteId} onChange={onSearchChange} search={search} />
        {statistics && (
          <div className='grid gap-5'>
            <div className='flex items-center gap-2' role='status'>
              <span>{t('pricingGroups.statisticsStatus')}</span>
              <DataStatusBadge status={statistics.data_status} />
            </div>
            <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-3'>
              {(
                [
                  [t('pricingGroups.metric.pricing'), statistics.total],
                  [t('pricingGroups.metric.groups'), statistics.group_total],
                  [t('pricingGroups.metric.missing'), statistics.missing],
                ] as const
              ).map(([label, value]) => (
                <div className='border-border p-4 sm:border-r' key={label}>
                  <dt className='text-muted-foreground text-xs'>{label}</dt>
                  <dd className='mt-1 text-xl font-semibold'>
                    <MetricValue value={value} />
                  </dd>
                </div>
              ))}
            </dl>
            <SiteBreakdown items={statistics.site_breakdown} />
            <TypedBreakdowns statistics={statistics} />
          </div>
        )}
        {search.tab === 'groups' && groupsQuery.data && (
          <div className='grid gap-3'>
            <div className='flex items-center gap-2' role='status'>
              <span>{t('pricingGroups.groupCompleteness')}</span>
              <DataStatusBadge status={groupsQuery.data.data_status} />
              <span className='text-muted-foreground text-xs'>
                {t('pricingGroups.asOf', {
                  time: timestamp(groupsQuery.data.as_of),
                })}
              </span>
            </div>
            <SiteBreakdown items={groupsQuery.data.site_breakdown} />
          </div>
        )}
        {search.tab === 'pricing' ? (
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
            onRetry={() => void pricingQuery.refetch()}
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(item) => (
              <article className='border-border bg-card grid gap-3 rounded-lg border p-4'>
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
        ) : (
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
            onRetry={() => void groupsQuery.refetch()}
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(item) => (
              <article className='border-border bg-card grid gap-3 rounded-lg border p-4'>
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
                <code>{item.ratio ?? '-'}</code>
                <div className='flex flex-wrap gap-2'>
                  <StateBadge state={item.remote_state} />
                  <DataStatusBadge status={item.data_status} />
                </div>
              </article>
            )}
            total={groupsQuery.data?.total ?? 0}
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
