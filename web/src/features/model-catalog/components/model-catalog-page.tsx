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
import {
  isIdString,
  isNonNegativeIdString,
  parseIdString,
  parseNonNegativeIdString,
} from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'

import {
  getModelCoverage,
  getSiteModelCoverage,
  listMissingModels,
  listModelCatalog,
  listSiteMissingModels,
  listSiteModelCatalog,
} from '../api'
import { buildModelCatalogExportRequest } from '../export-request'
import { modelCatalogKeys } from '../query-keys'
import type { ModelCatalogSearch } from '../search'
import type {
  MissingModelItem,
  ModelBinaryState,
  ModelCatalogItem,
  ModelCatalogQueryParams,
  ModelCoverageBreakdown,
  ModelCoverageMetric,
  ModelNameRule,
} from '../types'

function params(search: ModelCatalogSearch): ModelCatalogQueryParams {
  return {
    keyword: search.keyword || undefined,
    p: search.page,
    page_size: search.pageSize,
    site_ids: search.siteIds,
    statuses: search.statuses,
    sync_official: search.syncOfficial,
    vendor_id: search.vendorId,
  }
}

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
}: {
  global: boolean
  onChange: (changes: Partial<ModelCatalogSearch>) => void
  search: ModelCatalogSearch
}) {
  const { t } = useTranslation()
  const choices = (key: 'statuses' | 'syncOfficial', label: string) => (
    <fieldset className='grid gap-1'>
      <legend className='text-sm'>{label}</legend>
      <div className='flex gap-2'>
        {([0, 1] as const).map((value) => {
          const active = search[key].includes(value)
          return (
            <Button
              aria-pressed={active}
              key={value}
              onClick={() =>
                onChange({
                  [key]: active
                    ? search[key].filter((item) => item !== value)
                    : [...search[key], value],
                  page: 1,
                })
              }
              size='sm'
              type='button'
              variant={active ? 'secondary' : 'outline'}
            >
              {binaryText(value, t)}
            </Button>
          )
        })}
      </div>
    </fieldset>
  )
  return (
    <section
      aria-labelledby='model-catalog-filters-title'
      className='border-border bg-card grid gap-4 rounded-lg border p-4'
    >
      <div>
        <h2 className='font-medium' id='model-catalog-filters-title'>
          {t('modelCatalog.filters.title')}
        </h2>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t('modelCatalog.filters.description')}
        </p>
      </div>
      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <label className='grid gap-1 text-sm'>
          <span>{t('modelCatalog.filters.keyword')}</span>
          <Input
            onChange={(event) =>
              onChange({ keyword: event.target.value, page: 1 })
            }
            value={search.keyword}
          />
        </label>
        {global && (
          <label className='grid gap-1 text-sm'>
            <span>{t('modelCatalog.filters.siteIds')}</span>
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
        <label className='grid gap-1 text-sm'>
          <span>{t('modelCatalog.filters.vendorId')}</span>
          <Input
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
            value={search.vendorId ?? ''}
          />
        </label>
      </div>
      <div className='grid gap-3 sm:grid-cols-2'>
        {choices('statuses', t('modelCatalog.filters.statuses'))}
        {choices('syncOfficial', t('modelCatalog.filters.syncOfficial'))}
      </div>
    </section>
  )
}

function CoverageGrid({ metric }: { metric: ModelCoverageMetric }) {
  const { t } = useTranslation()
  const values = [
    [t('modelCatalog.metric.catalog'), metric.catalog_models],
    [t('modelCatalog.metric.covered'), metric.exact_covered_models],
    [t('modelCatalog.metric.missing'), metric.exact_missing_models],
    [t('modelCatalog.metric.mappings'), metric.channel_mappings],
  ] as const
  return (
    <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 xl:grid-cols-4'>
      {values.map(([label, value]) => (
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

function CoverageBreakdown({
  items,
  title,
}: {
  items: ModelCoverageBreakdown[]
  title: string
}) {
  const { t } = useTranslation()
  return (
    <section className='grid gap-3'>
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
  const validSiteId = siteId == null || isIdString(siteId)
  const currentParams = useMemo(() => params(search), [search])
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
    enabled: validSiteId && search.tab === 'coverage',
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? getSiteModelCoverage(parseIdString(siteId), currentParams)
        : getModelCoverage(currentParams),
    queryKey:
      siteId && isIdString(siteId)
        ? modelCatalogKeys.site(siteId, 'coverage', currentParams)
        : modelCatalogKeys.global('coverage', currentParams),
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
          <div className='min-w-44'>
            <span className='font-medium'>{row.original.model_name}</span>
            <span className='text-muted-foreground block text-xs'>
              {row.original.site_name} · {row.original.site_id}
            </span>
            <code className='text-muted-foreground block text-xs'>
              {row.original.remote_id}
            </code>
          </div>
        ),
        header: t('modelCatalog.identity'),
        id: 'identity',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-52 gap-1 text-xs'>
            <span className='whitespace-pre-wrap'>
              {row.original.description || '-'}
            </span>
            <code className='border-border bg-muted/50 rounded border p-1.5 break-all'>
              {row.original.icon || '-'}
            </code>
            <span>{row.original.tags || '-'}</span>
          </div>
        ),
        header: t('modelCatalog.metadata'),
        id: 'metadata',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-32 gap-1 text-xs'>
            <span>
              {t('modelCatalog.vendorValue', { value: row.original.vendor_id })}
            </span>
            <BinaryBadge value={row.original.status} />
            <span>
              {t('modelCatalog.syncValue', {
                value: binaryText(row.original.sync_official, t),
              })}
            </span>
            <span>{ruleText(row.original.name_rule, t)}</span>
          </div>
        ),
        header: t('modelCatalog.policy'),
        id: 'policy',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-36 gap-1 text-xs'>
            <span>
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
          <div className='grid min-w-40 gap-1 text-xs'>
            <span>{timestamp(row.original.created_time)}</span>
            <span>{timestamp(row.original.updated_time)}</span>
          </div>
        ),
        header: t('modelCatalog.timestamps'),
        id: 'timestamps',
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
    ['catalog', t('modelCatalog.tabs.catalog')],
    ['coverage', t('modelCatalog.tabs.coverage')],
    ['missing', t('modelCatalog.tabs.missing')],
  ] as const
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
      title={siteId ? t('modelCatalog.siteTitle') : t('modelCatalog.title')}
    >
      <div className='grid min-w-0 gap-6'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('modelCatalog.backToSite')}
          </DetailBackLink>
        )}
        <section
          className='border-primary/30 bg-primary/5 rounded-lg border p-4'
          role='note'
        >
          <p className='font-medium'>{t('modelCatalog.boundary.title')}</p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('modelCatalog.boundary.description')}
          </p>
        </section>
        <div
          aria-label={t('modelCatalog.tabs.label')}
          className='flex flex-wrap gap-2'
          role='tablist'
        >
          {tabs.map(([tab, label]) => (
            <Button
              aria-selected={search.tab === tab}
              key={tab}
              onClick={() => onSearchChange({ page: 1, tab })}
              role='tab'
              variant={search.tab === tab ? 'secondary' : 'outline'}
            >
              {label}
            </Button>
          ))}
        </div>
        <Filters global={!siteId} onChange={onSearchChange} search={search} />
        {search.tab === 'catalog' && (
          <DataTable
            ariaLabel={t('modelCatalog.table')}
            columns={catalogColumns}
            data={catalogQuery.data?.items ?? []}
            emptyDescription={t('modelCatalog.emptyDescription')}
            emptyTitle={t('modelCatalog.empty')}
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
              <article className='border-border bg-card grid gap-3 rounded-lg border p-4'>
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
        {search.tab === 'coverage' && coverageQuery.data && (
          <div className='grid gap-6'>
            <div className='flex items-center gap-2' role='status'>
              <span>{t('modelCatalog.coverageStatus')}</span>
              <DataStatusBadge status={coverageQuery.data.data_status} />
            </div>
            <CoverageGrid metric={coverageQuery.data} />
            <div className='grid gap-6 xl:grid-cols-3'>
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
          </div>
        )}
        {search.tab === 'coverage' && coverageQuery.isError && (
          <Button
            onClick={() => void coverageQuery.refetch()}
            variant='outline'
          >
            {t('common.retry')}
          </Button>
        )}
        {search.tab === 'missing' && (
          <div className='grid gap-4'>
            <section
              className='border-warning/30 bg-warning/5 rounded-lg border p-4'
              role='note'
            >
              {t('modelCatalog.exactBoundary')}
            </section>
            <DataTable
              ariaLabel={t('modelCatalog.missingTable')}
              columns={missingColumns}
              data={missingQuery.data?.items ?? []}
              emptyDescription={t('modelCatalog.missingEmptyDescription')}
              emptyTitle={t('modelCatalog.missingEmpty')}
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
                <article className='border-border bg-card grid gap-2 rounded-lg border p-4'>
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
