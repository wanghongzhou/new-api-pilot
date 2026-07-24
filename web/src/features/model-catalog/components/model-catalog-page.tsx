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
import {
  isIdString,
  isNonNegativeIdString,
  parseIdString,
  parseNonNegativeIdString,
} from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import { hasFilterChanges } from '@/lib/filter-state'

import {
  getModelCoverage,
  getSiteModelCoverage,
  listMissingModels,
  listModelCatalog,
  listSiteMissingModels,
  listSiteModelCatalog,
} from '../api'
import {
  getMissingModelEmptyState,
  getModelCatalogEmptyState,
} from '../empty-state'
import { buildModelCatalogExportRequest } from '../export-request'
import { modelCatalogKeys } from '../query-keys'
import {
  buildModelCatalogQueryParams,
  buildModelCatalogSearch,
  changeModelCatalogTab,
  hasMissingIncompatibleFilters,
  type ModelCatalogSearch,
} from '../search'
import type {
  MissingModelItem,
  ModelBinaryState,
  ModelCatalogItem,
  ModelCoverageBreakdown,
  ModelCoverageMetric,
  ModelNameRule,
} from '../types'

function timestamp(value: number | null) {
  if (value == null || value <= 0) return '-'
  return fromUnixSeconds(value).format('YYYY-MM-DD HH:mm:ss')
}

function binaryText(value: ModelBinaryState, t: (key: string) => string) {
  return value === 1
    ? t('modelCatalog.binary.enabled')
    : t('modelCatalog.binary.disabled')
}

function ruleText(value: ModelNameRule, t: (key: string) => string) {
  if (value === 0) return t('modelCatalog.rule.exact')
  if (value === 1) return t('modelCatalog.rule.prefix')
  if (value === 2) return t('modelCatalog.rule.contains')
  return t('modelCatalog.rule.suffix')
}

function BinaryBadge({ value }: { value: ModelBinaryState }) {
  const { t } = useTranslation()
  return (
    <Badge variant={value === 1 ? 'success' : 'neutral'}>
      {binaryText(value, t)}
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
  onChange: (changes: Partial<ModelCatalogSearch>) => void
  search: ModelCatalogSearch
  sites: SiteListItem[]
}) {
  const { t } = useTranslation()
  const reset = buildModelCatalogSearch({
    pageSize: search.pageSize,
    tab: search.tab,
  })
  const supportsCatalogFilters = search.tab !== 'missing'
  const hasActiveFilters = hasFilterChanges(search, reset, [
    'keyword',
    'siteIds',
    ...(supportsCatalogFilters
      ? (['statuses', 'syncOfficial', 'vendorId'] as const)
      : []),
  ])
  const binaryOptions = ([0, 1] as const).map((value) => ({
    label: binaryText(value, t),
    value: String(value),
  }))
  const binaryValue = (values: ModelBinaryState[]) =>
    values.length === 1 ? String(values[0]) : ''
  const updateBinary = (key: 'statuses' | 'syncOfficial', value: string) =>
    onChange({
      [key]: value === '0' || value === '1' ? [Number(value)] : [],
      page: 1,
    })
  return (
    <section
      aria-label={t('modelCatalog.filters.title')}
      className='flex min-w-0 flex-wrap items-center gap-2'
    >
      <label className='relative min-w-48 flex-1 sm:max-w-72'>
        <span className='sr-only'>{t('modelCatalog.filters.keyword')}</span>
        <HugeiconsIcon
          className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2'
          icon={Search01Icon}
          size={15}
          strokeWidth={2}
        />
        <Input
          aria-label={t('modelCatalog.filters.keyword')}
          className='h-8 pl-8'
          onChange={(event) =>
            onChange({ keyword: event.target.value, page: 1 })
          }
          placeholder={t('modelCatalog.filters.keywordPlaceholder')}
          value={search.keyword}
        />
      </label>
      {global && (
        <FacetedFilter
          clearLabel={t('modelCatalog.filters.allSites')}
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
          title={t('modelCatalog.filters.site')}
          value={search.siteIds.length === 1 ? search.siteIds[0] : ''}
        />
      )}
      {supportsCatalogFilters && (
        <>
          <FacetedFilter
            clearLabel={t('modelCatalog.filters.allStatuses')}
            onChange={(value) => updateBinary('statuses', value)}
            options={binaryOptions}
            title={t('modelCatalog.filters.statuses')}
            value={binaryValue(search.statuses)}
          />
          <FacetedFilter
            clearLabel={t('modelCatalog.filters.allSyncStates')}
            onChange={(value) => updateBinary('syncOfficial', value)}
            options={binaryOptions}
            title={t('modelCatalog.filters.syncOfficial')}
            value={binaryValue(search.syncOfficial)}
          />
          <label>
            <span className='sr-only'>
              {t('modelCatalog.filters.vendorId')}
            </span>
            <Input
              aria-label={t('modelCatalog.filters.vendorId')}
              className='h-8 w-32'
              inputMode='numeric'
              onChange={(event) => {
                const value = event.target.value
                onChange({
                  page: 1,
                  vendorId: isNonNegativeIdString(value)
                    ? parseNonNegativeIdString(value)
                    : undefined,
                })
              }}
              placeholder={t('modelCatalog.filters.vendorId')}
              value={search.vendorId ?? ''}
            />
          </label>
        </>
      )}
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

function CoverageGrid({
  metric,
  missingValue,
}: {
  metric?: ModelCoverageMetric
  missingValue?: ModelCoverageMetric['exact_missing_models']
}) {
  const { t } = useTranslation()
  const values = [
    {
      icon: Database01Icon,
      label: t('modelCatalog.metric.catalog'),
      value: metric?.catalog_models,
    },
    {
      icon: Chart01Icon,
      label: t('modelCatalog.metric.covered'),
      value: metric?.exact_covered_models,
    },
    {
      icon: Alert02Icon,
      label: t('modelCatalog.metric.missing'),
      value: missingValue,
    },
    {
      icon: Chart01Icon,
      label: t('modelCatalog.metric.mappings'),
      value: metric?.channel_mappings,
    },
  ] as const
  return (
    <dl className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
      {values.map(({ icon, label, value }) => (
        <div
          className='bg-card text-card-foreground ring-foreground/10 flex items-center gap-3 rounded-xl p-4 ring-1'
          key={label}
        >
          <span className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg'>
            <HugeiconsIcon icon={icon} size={18} strokeWidth={2} />
          </span>
          <div className='min-w-0'>
            <dt className='text-muted-foreground truncate text-xs'>{label}</dt>
            <dd className='mt-0.5 text-2xl font-semibold tracking-tight'>
              {value == null ? '-' : <MetricValue value={value} />}
            </dd>
          </div>
        </div>
      ))}
    </dl>
  )
}

function TabPurpose({
  status,
  tab,
}: {
  status?: ModelCoverageBreakdown['data_status']
  tab: ModelCatalogSearch['tab']
}) {
  const { t } = useTranslation()
  let content = {
    description: t('modelCatalog.purpose.catalogDescription'),
    icon: Database01Icon,
    title: t('modelCatalog.purpose.catalogTitle'),
  }
  if (tab === 'coverage') {
    content = {
      description: t('modelCatalog.purpose.coverageDescription'),
      icon: Chart01Icon,
      title: t('modelCatalog.purpose.coverageTitle'),
    }
  } else if (tab === 'missing') {
    content = {
      description: t('modelCatalog.purpose.missingDescription'),
      icon: Alert02Icon,
      title: t('modelCatalog.purpose.missingTitle'),
    }
  }
  return (
    <section className='border-border bg-muted/30 flex items-start gap-3 rounded-xl border p-4'>
      <span className='bg-background text-muted-foreground ring-foreground/10 flex size-9 shrink-0 items-center justify-center rounded-lg ring-1'>
        <HugeiconsIcon icon={content.icon} size={18} strokeWidth={2} />
      </span>
      <div className='min-w-0 flex-1'>
        <div className='flex flex-wrap items-center gap-2'>
          <h2 className='font-medium'>{content.title}</h2>
          {status && <DataStatusBadge status={status} />}
        </div>
        <p className='text-muted-foreground mt-1 text-sm'>
          {content.description}
        </p>
      </div>
    </section>
  )
}

function CoverageBreakdown({
  items,
  title,
}: {
  items: ModelCoverageBreakdown[]
  title: string
}) {
  const { t } = useTranslation()
  return (
    <section className='grid min-w-0 content-start gap-3'>
      <h3 className='font-semibold'>{title}</h3>
      {items.length === 0 ? (
        <p className='text-muted-foreground text-sm'>{t('common.none')}</p>
      ) : (
        <div className='grid gap-2'>
          {items.map((item) => (
            <article
              className='border-border grid gap-2 rounded-lg border p-3'
              key={`${item.site_id}:${item.dimension_id}`}
            >
              <div className='flex items-start justify-between gap-2'>
                <div>
                  <p className='font-medium'>{item.dimension_name || '-'}</p>
                  <code className='text-muted-foreground text-xs'>
                    {item.dimension_id || '-'}
                  </code>
                  {item.site_id !== '0' && (
                    <p className='text-muted-foreground text-xs'>
                      {item.site_name} · {item.site_id}
                    </p>
                  )}
                </div>
                <DataStatusBadge status={item.data_status} />
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('modelCatalog.breakdown.values', {
                  catalog: item.catalog_models,
                  covered: item.exact_covered_models,
                  mappings: item.channel_mappings,
                  missing: item.exact_missing_models,
                })}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t('modelCatalog.asOf', { time: timestamp(item.as_of) })}
              </p>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

export function ModelCatalogPage({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<ModelCatalogSearch>) => void
  search: ModelCatalogSearch
  siteId?: string
}) {
  const { t } = useTranslation()
  const [initialJob, setInitialJob] = useState<StatisticsExportJobItem>()
  const canonicalizedSearch = useRef(false)
  const validSiteId = siteId == null || isIdString(siteId)
  const currentParams = useMemo(
    () => buildModelCatalogQueryParams(search),
    [search]
  )
  const coverageParams = useMemo(
    () => buildModelCatalogQueryParams(buildModelCatalogSearch({}), 'coverage'),
    []
  )
  const hasAnyViewFilter =
    search.keyword !== '' ||
    search.siteIds.length > 0 ||
    hasMissingIncompatibleFilters(search)
  const hasCoverageIncompatibleState =
    hasAnyViewFilter || search.page !== 1 || search.pageSize !== 20
  useEffect(() => {
    if (search.tab === 'coverage' && hasCoverageIncompatibleState) {
      onSearchChange(changeModelCatalogTab('coverage'))
    } else if (
      search.tab === 'missing' &&
      hasMissingIncompatibleFilters(search)
    ) {
      onSearchChange(changeModelCatalogTab('missing'))
    } else if (!canonicalizedSearch.current) {
      canonicalizedSearch.current = true
      onSearchChange({})
    }
  }, [hasCoverageIncompatibleState, onSearchChange, search])
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
  const catalogQuery = useQuery({
    enabled: validSiteId && search.tab === 'catalog',
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? listSiteModelCatalog(parseIdString(siteId), currentParams)
        : listModelCatalog(currentParams),
    queryKey:
      siteId && isIdString(siteId)
        ? modelCatalogKeys.site(siteId, 'catalog', currentParams)
        : modelCatalogKeys.global('catalog', currentParams),
  })
  const coverageQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? getSiteModelCoverage(parseIdString(siteId), coverageParams)
        : getModelCoverage(coverageParams),
    queryKey:
      siteId && isIdString(siteId)
        ? modelCatalogKeys.site(siteId, 'coverage', coverageParams)
        : modelCatalogKeys.global('coverage', coverageParams),
  })
  const missingQuery = useQuery({
    enabled: validSiteId && search.tab === 'missing',
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? listSiteMissingModels(parseIdString(siteId), currentParams)
        : listMissingModels(currentParams),
    queryKey:
      siteId && isIdString(siteId)
        ? modelCatalogKeys.site(siteId, 'missing', currentParams)
        : modelCatalogKeys.global('missing', currentParams),
  })
  const exportMutation = useMutation({
    mutationFn: (format: StatisticsExportFormat) =>
      createStatisticsExport(
        buildModelCatalogExportRequest(
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
  const catalogColumns = useMemo<ColumnDef<ModelCatalogItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='max-w-96 min-w-56'>
            <p className='font-mono font-medium break-all'>
              {row.original.model_name}
            </p>
            <p className='text-muted-foreground mt-1 line-clamp-2 text-xs'>
              {row.original.description || '-'}
            </p>
          </div>
        ),
        header: t('modelCatalog.model'),
        id: 'model',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-36 gap-0.5 text-xs'>
            <span className='font-medium'>{row.original.site_name}</span>
            <span className='text-muted-foreground'>
              {t('modelCatalog.siteIdValue', { value: row.original.site_id })}
            </span>
            <span className='text-muted-foreground'>
              {t('modelCatalog.remoteIdValue', {
                value: row.original.remote_id,
              })}
            </span>
          </div>
        ),
        header: t('modelCatalog.siteIdentity'),
        id: 'siteIdentity',
      },
      {
        cell: ({ row }) => (
          <div className='flex min-w-28 flex-wrap gap-1'>
            <BinaryBadge value={row.original.status} />
            <Badge
              variant={row.original.sync_official === 1 ? 'success' : 'neutral'}
            >
              {row.original.sync_official === 1
                ? t('modelCatalog.sync.official')
                : t('modelCatalog.sync.manual')}
            </Badge>
          </div>
        ),
        header: t('modelCatalog.state'),
        id: 'state',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-40 gap-1 text-xs'>
            <div className='flex flex-wrap gap-1'>
              <Badge variant='outline'>
                {t('modelCatalog.vendorValue', {
                  value: row.original.vendor_id,
                })}
              </Badge>
              <Badge variant='outline'>
                {ruleText(row.original.name_rule, t)}
              </Badge>
            </div>
            <span className='text-muted-foreground line-clamp-1'>
              {row.original.tags || '-'}
            </span>
            <code className='text-muted-foreground line-clamp-1 break-all'>
              {row.original.icon || '-'}
            </code>
          </div>
        ),
        header: t('modelCatalog.metadata'),
        id: 'metadata',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-32 gap-1 text-xs'>
            <span className='font-medium'>
              {t('modelCatalog.channelsValue', {
                value: row.original.covered_channels,
              })}
            </span>
            <span>
              {t('modelCatalog.groupsValue', {
                value: row.original.covered_groups,
              })}
            </span>
            <DataStatusBadge status={row.original.data_status} />
          </div>
        ),
        header: t('modelCatalog.coverage'),
        id: 'coverage',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-36 gap-1 text-xs'>
            <span>{timestamp(row.original.updated_time)}</span>
            <span className='text-muted-foreground'>
              {t('modelCatalog.createdValue', {
                value: timestamp(row.original.created_time),
              })}
            </span>
          </div>
        ),
        header: t('modelCatalog.updatedAt'),
        id: 'updatedAt',
      },
    ],
    [t]
  )
  const missingColumns = useMemo<ColumnDef<MissingModelItem, unknown>[]>(
    () => [
      { accessorKey: 'model_name', header: t('modelCatalog.modelName') },
      {
        cell: ({ row }) =>
          `${row.original.site_name} · ${row.original.site_id}`,
        header: t('modelCatalog.site'),
        id: 'site',
      },
      {
        cell: ({ row }) => (
          <div className='grid gap-1 text-xs'>
            <span>{row.original.channel_name || '-'}</span>
            <code>{row.original.remote_channel_id}</code>
          </div>
        ),
        header: t('modelCatalog.channel'),
        id: 'channel',
      },
      { accessorKey: 'group', header: t('modelCatalog.group') },
      {
        cell: ({ row }) => (
          <div className='grid gap-1'>
            <DataStatusBadge status={row.original.data_status} />
            <span className='text-muted-foreground text-xs'>
              {timestamp(row.original.as_of)}
            </span>
          </div>
        ),
        header: t('common.status'),
        id: 'status',
      },
    ],
    [t]
  )
  const tabs = [
    {
      count: coverageQuery.data?.catalog_models,
      icon: Database01Icon,
      label: t('modelCatalog.tabs.catalog'),
      value: 'catalog',
    },
    {
      count: coverageQuery.data?.exact_covered_models,
      icon: Chart01Icon,
      label: t('modelCatalog.tabs.coverage'),
      value: 'coverage',
    },
    {
      count: coverageQuery.data?.exact_missing_models,
      icon: Alert02Icon,
      label: t('modelCatalog.tabs.missing'),
      value: 'missing',
    },
  ] as const
  const catalogEmptyState = getModelCatalogEmptyState(
    catalogQuery.data?.data_status,
    search
  )
  const missingEmptyState = getMissingModelEmptyState(
    missingQuery.data?.data_status,
    search
  )
  let activeDataStatus = coverageQuery.data?.data_status
  if (search.tab === 'catalog') {
    activeDataStatus = catalogQuery.data?.data_status
  }
  if (search.tab === 'missing') {
    activeDataStatus = missingQuery.data?.data_status
  }
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
          {t('modelCatalog.export', { format: format.toUpperCase() })}
        </Button>
      ))}
      description={
        siteId
          ? t('modelCatalog.siteDescription', { id: siteId })
          : t('modelCatalog.description')
      }
      fixedContent
      title={siteId ? t('modelCatalog.siteTitle') : t('modelCatalog.title')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('modelCatalog.backToSite')}
          </DetailBackLink>
        )}
        <CoverageGrid
          metric={coverageQuery.data}
          missingValue={coverageQuery.data?.exact_missing_models}
        />
        <Tabs
          onValueChange={(tab) =>
            onSearchChange(
              changeModelCatalogTab(tab as ModelCatalogSearch['tab'])
            )
          }
          value={search.tab}
        >
          <TabsList
            aria-label={t('modelCatalog.tabs.label')}
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
        <TabPurpose status={activeDataStatus} tab={search.tab} />
        {search.tab !== 'coverage' && (
          <Filters
            global={!siteId}
            onChange={onSearchChange}
            search={search}
            sites={sitesQuery.data?.items ?? []}
          />
        )}
        {search.tab === 'catalog' && (
          <DataTable
            ariaLabel={t('modelCatalog.table')}
            columns={catalogColumns}
            data={catalogQuery.data?.items ?? []}
            emptyDescription={t(
              dynamicI18nKey('modelCatalog', catalogEmptyState.descriptionKey)
            )}
            emptyTitle={t(
              dynamicI18nKey('modelCatalog', catalogEmptyState.titleKey)
            )}
            error={!validSiteId || catalogQuery.isError}
            fetching={catalogQuery.isFetching}
            loading={catalogQuery.isPending}
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={() => void catalogQuery.refetch()}
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(item) => (
              <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
                <div className='flex items-start justify-between gap-2'>
                  <div className='min-w-0'>
                    <p className='font-medium'>{item.model_name}</p>
                    <span className='text-muted-foreground text-xs'>
                      {item.site_name} · {item.site_id}
                    </span>
                  </div>
                  <BinaryBadge value={item.status} />
                </div>
                <p className='text-sm'>{item.description || '-'}</p>
                <code className='border-border bg-muted/50 rounded border p-2 text-xs break-all'>
                  {item.icon || '-'}
                </code>
                <dl className='grid grid-cols-2 gap-3 text-sm'>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('modelCatalog.vendor')}
                    </dt>
                    <dd>{item.vendor_id}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('modelCatalog.rule')}
                    </dt>
                    <dd>{ruleText(item.name_rule, t)}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('modelCatalog.channels')}
                    </dt>
                    <dd>{item.covered_channels}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('modelCatalog.groups')}
                    </dt>
                    <dd>{item.covered_groups}</dd>
                  </div>
                </dl>
              </article>
            )}
            total={catalogQuery.data?.total ?? 0}
          />
        )}
        {search.tab === 'coverage' && (
          <div className='min-h-0 flex-1 overflow-y-auto'>
            {coverageQuery.isPending && (
              <div
                aria-hidden='true'
                className='border-border bg-muted/40 h-64 animate-pulse rounded-xl border'
              />
            )}
            {coverageQuery.data && (
              <div className='grid gap-4 xl:grid-cols-3'>
                <CoverageBreakdown
                  items={coverageQuery.data.site_breakdown}
                  title={t('modelCatalog.breakdown.site')}
                />
                <CoverageBreakdown
                  items={coverageQuery.data.vendor_breakdown}
                  title={t('modelCatalog.breakdown.vendor')}
                />
                <CoverageBreakdown
                  items={coverageQuery.data.status_breakdown}
                  title={t('modelCatalog.breakdown.status')}
                />
              </div>
            )}
            {coverageQuery.isError && (
              <Button
                onClick={() => void coverageQuery.refetch()}
                variant='outline'
              >
                {t('common.retry')}
              </Button>
            )}
          </div>
        )}
        {search.tab === 'missing' && (
          <DataTable
            ariaLabel={t('modelCatalog.missingTable')}
            columns={missingColumns}
            data={missingQuery.data?.items ?? []}
            emptyDescription={t(
              dynamicI18nKey('modelCatalog', missingEmptyState.descriptionKey)
            )}
            emptyTitle={t(
              dynamicI18nKey('modelCatalog', missingEmptyState.titleKey)
            )}
            error={!validSiteId || missingQuery.isError}
            fetching={missingQuery.isFetching}
            loading={missingQuery.isPending}
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={() => void missingQuery.refetch()}
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(item) => (
              <article className='bg-card text-card-foreground ring-foreground/10 grid gap-2 rounded-xl p-4 ring-1'>
                <div className='flex items-start justify-between gap-2'>
                  <p className='font-medium'>{item.model_name}</p>
                  <DataStatusBadge status={item.data_status} />
                </div>
                <p className='text-muted-foreground text-xs'>
                  {item.site_name} · {item.site_id}
                </p>
                <p className='text-sm'>
                  {item.channel_name || '-'} · {item.remote_channel_id}
                </p>
                <p className='text-sm'>
                  {t('modelCatalog.groupValue', { value: item.group || '-' })}
                </p>
              </article>
            )}
            total={missingQuery.data?.total ?? 0}
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
