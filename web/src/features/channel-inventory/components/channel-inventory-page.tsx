import {
  Alert02Icon,
  ArrowLeft01Icon,
  Chart01Icon,
  Database01Icon,
  FileExportIcon,
  TableIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import Decimal from 'decimal.js'
import { useMemo, useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataStatusBadge } from '@/components/data/data-status'
import { DebouncedInput } from '@/components/data/debounced-input'
import { FacetedFilter } from '@/components/data/faceted-filter'
import { FilterPanel } from '@/components/data/filter-panel'
import { InventoryTrendChart } from '@/components/data/inventory-trend-chart'
import { MetricValue } from '@/components/data/metric-value'
import { MultiFacetedFilter } from '@/components/data/multi-faceted-filter'
import { QueryStateAlert } from '@/components/data/query-state-alert'
import { ErrorState } from '@/components/error-state'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { LoadingState } from '@/components/loading-state'
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
import { useRetainedQueryData } from '@/hooks/use-retained-query-data'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import {
  isDecimalString,
  isIdString,
  isMetricString,
  parseDecimalString,
  parseIdString,
  parseMetricString,
} from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'
import {
  formatDecimalDisplayValue,
  formatMetricDisplayValue,
} from '@/lib/display-value'
import { hasFilterChanges } from '@/lib/filter-state'

import {
  getChannelInventoryStatistics,
  getSiteChannelInventoryStatistics,
  listChannelInventory,
  listSiteChannelInventory,
} from '../api'
import { buildChannelInventoryExportRequest } from '../export-request'
import { channelInventoryKeys } from '../query-keys'
import {
  buildChannelInventorySearch,
  changeChannelInventoryTab,
  type ChannelInventorySearch,
} from '../search'
import type {
  ChannelInventoryBreakdown,
  ChannelInventoryItem,
  ChannelInventoryMetric,
  ChannelInventoryQueryParams,
  ChannelInventoryState,
  ChannelInventoryStatisticsQueryParams,
  ChannelInventoryTrendPoint,
} from '../types'

const statusValues = [0, 1, 2, 3] as const
const stateValues: ChannelInventoryState[] = ['normal', 'missing']
const collapsedModelCount = 6

function availabilityText(value: string) {
  try {
    const percentage = new Decimal(value).mul(100)
    return `${formatDecimalDisplayValue(percentage.toString(), 2)}%`
  } catch {
    return '-'
  }
}

function qualityMetricText(
  value: string,
  kind: 'decimal' | 'metric' | 'percentage'
) {
  if (kind === 'metric') return <MetricValue value={value} />
  if (kind === 'percentage') return availabilityText(value)
  return formatDecimalDisplayValue(value)
}

function ChannelModelList({ models }: { models: string }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const items = [
    ...new Set(
      models
        .split(',')
        .map((model) => model.trim())
        .filter(Boolean)
    ),
  ]

  if (items.length === 0) return <span>-</span>

  const visibleItems = expanded ? items : items.slice(0, collapsedModelCount)
  const remaining = items.length - visibleItems.length
  return (
    <div className='flex max-w-64 min-w-48 flex-wrap items-start gap-1 whitespace-normal'>
      {visibleItems.map((model) => (
        <Badge
          className='h-auto max-w-full py-0.5 text-left break-all whitespace-normal'
          key={model}
          title={model}
          variant='outline'
        >
          {model}
        </Badge>
      ))}
      {items.length > collapsedModelCount && (
        <Button
          aria-expanded={expanded}
          className='h-auto px-1.5 py-0.5 text-xs'
          onClick={() => setExpanded((value) => !value)}
          size='sm'
          type='button'
          variant='ghost'
        >
          {expanded
            ? t('common.collapse')
            : t('channelInventory.modelsMore', { count: remaining })}
        </Button>
      )}
    </div>
  )
}

function listParams(
  search: ChannelInventorySearch
): ChannelInventoryQueryParams {
  return {
    groups: search.groups,
    keyword: search.keyword || undefined,
    max_balance: search.maxBalance,
    max_response_time_ms: search.maxResponseTime,
    min_balance: search.minBalance,
    min_response_time_ms: search.minResponseTime,
    p: search.page,
    page_size: search.pageSize,
    site_ids: search.siteIds,
    states: search.states,
    statuses: search.statuses,
    tags: search.tags,
    types: search.types,
  }
}

function statisticsParams(
  search: ChannelInventorySearch
): ChannelInventoryStatisticsQueryParams {
  return {
    end_timestamp: search.end,
    groups: search.groups,
    site_ids: search.siteIds,
    start_timestamp: search.start,
    statuses: search.statuses,
    tags: search.tags,
    types: search.types,
  }
}

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

function statusText(value: number, t: (key: string) => string) {
  if (value === 0) return t('channelInventory.status.unknown')
  if (value === 1) return t('channelInventory.status.enabled')
  if (value === 2) return t('channelInventory.status.manuallyDisabled')
  if (value === 3) return t('channelInventory.status.autoDisabled')
  return t('common.unknown')
}

function purposeText(
  tab: ChannelInventorySearch['tab'],
  t: (key: string) => string
) {
  if (tab === 'trend') {
    return {
      description: t('channelInventory.purpose.trendDescription'),
      title: t('channelInventory.purpose.trendTitle'),
    }
  }
  if (tab === 'dimensions') {
    return {
      description: t('channelInventory.purpose.dimensionsDescription'),
      title: t('channelInventory.purpose.dimensionsTitle'),
    }
  }
  if (tab === 'sites') {
    return {
      description: t('channelInventory.purpose.sitesDescription'),
      title: t('channelInventory.purpose.sitesTitle'),
    }
  }
  return {
    description: t('channelInventory.purpose.listDescription'),
    title: t('channelInventory.purpose.listTitle'),
  }
}

function ChannelStateBadge({ state }: { state: ChannelInventoryState }) {
  const { t } = useTranslation()
  return (
    <Badge variant={state === 'normal' ? 'success' : 'warning'}>
      {state === 'normal'
        ? t('channelInventory.state.normal')
        : t('channelInventory.state.missing')}
    </Badge>
  )
}

function MetricGrid({ metric }: { metric: ChannelInventoryMetric }) {
  const { t } = useTranslation()
  const primary = [
    [
      Database01Icon,
      t('channelInventory.metric.channelCount'),
      metric.channel_count,
    ],
    [
      Chart01Icon,
      t('channelInventory.metric.available'),
      metric.available_count,
    ],
    [
      Alert02Icon,
      t('channelInventory.metric.unavailable'),
      metric.unavailable_count,
    ],
    [Alert02Icon, t('channelInventory.metric.missing'), metric.missing_count],
  ] as const
  const quality = [
    [t('channelInventory.metric.balance'), metric.balance_total, 'decimal'],
    [t('channelInventory.metric.usedQuota'), metric.used_quota, 'metric'],
    [
      t('channelInventory.metric.responseAvg'),
      metric.response_time_avg_ms,
      'decimal',
    ],
    [
      t('channelInventory.metric.responseMax'),
      metric.response_time_max_ms,
      'metric',
    ],
    [
      t('channelInventory.metric.availability'),
      metric.availability_rate,
      'percentage',
    ],
  ] as const
  return (
    <div className='grid gap-3'>
      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        {primary.map(([icon, label, value]) => (
          <div
            className='bg-card text-card-foreground ring-foreground/10 flex min-w-0 items-center gap-3 rounded-xl p-4 ring-1'
            key={label}
          >
            <span className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg'>
              <HugeiconsIcon icon={icon} size={18} strokeWidth={2} />
            </span>
            <dl className='min-w-0'>
              <dt className='text-muted-foreground truncate text-xs'>
                {label}
              </dt>
              <dd className='mt-0.5 text-2xl font-semibold tracking-tight'>
                <MetricValue value={value} />
              </dd>
            </dl>
          </div>
        ))}
      </div>
      <dl className='border-border bg-muted/20 grid gap-x-6 gap-y-2 rounded-xl border px-4 py-3 sm:grid-cols-2 xl:grid-cols-5'>
        {quality.map(([label, value, kind]) => (
          <div
            className='flex items-baseline justify-between gap-3'
            key={label}
          >
            <dt className='text-muted-foreground text-xs'>{label}</dt>
            <dd className='font-mono text-sm font-medium' title={value}>
              {qualityMetricText(value, kind)}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function MultiChoice({
  label,
  onChange,
  options,
  selected,
}: {
  label: string
  onChange: (values: Array<number | string>) => void
  options: ReadonlyArray<{ label: string; value: number | string }>
  selected: readonly (number | string)[]
}) {
  return (
    <fieldset className='grid gap-1'>
      <legend className='text-sm'>{label}</legend>
      <div className='flex min-h-10 flex-wrap gap-1.5'>
        {options.map((option) => {
          const active = selected.includes(option.value)
          return (
            <Button
              aria-pressed={active}
              key={String(option.value)}
              onClick={() =>
                onChange(
                  active
                    ? selected.filter((value) => value !== option.value)
                    : [...selected, option.value]
                )
              }
              size='sm'
              type='button'
              variant={active ? 'secondary' : 'outline'}
            >
              {option.label}
            </Button>
          )
        })}
      </div>
    </fieldset>
  )
}

function commaNumbers(value: string, maximum: number) {
  return [...new Set(value.split(',').map((item) => Number(item.trim())))]
    .filter((item) => Number.isInteger(item) && item >= 0 && item <= maximum)
    .sort((left, right) => left - right)
}

function InventoryFilters({
  global,
  onChange,
  search,
  sites,
}: {
  global: boolean
  onChange: (changes: Partial<ChannelInventorySearch>) => void
  search: ChannelInventorySearch
  sites: SiteListItem[]
}) {
  const { t } = useTranslation()
  const decimalChange =
    (key: 'maxBalance' | 'minBalance') =>
    (event: ChangeEvent<HTMLInputElement>) => {
      const value = event.target.value
      if (value === '') onChange({ [key]: undefined, page: 1 })
      else if (isDecimalString(value)) {
        onChange({ [key]: parseDecimalString(value), page: 1 })
      }
    }
  const metricChange =
    (key: 'maxResponseTime' | 'minResponseTime') =>
    (event: ChangeEvent<HTMLInputElement>) => {
      const value = event.target.value
      if (value === '') onChange({ [key]: undefined, page: 1 })
      else if (isMetricString(value) && !value.startsWith('-')) {
        onChange({ [key]: parseMetricString(value), page: 1 })
      }
    }
  const stringList =
    (key: 'groups' | 'tags') => (event: ChangeEvent<HTMLInputElement>) =>
      onChange({
        [key]: event.target.value
          .split(',')
          .map((value) => value.trim())
          .filter(Boolean),
        page: 1,
      })
  const reset = buildChannelInventorySearch({
    pageSize: search.pageSize,
    tab: search.tab,
    trendPageSize: search.trendPageSize,
    trendView: search.trendView,
  })
  const advancedCount = [
    search.types.length > 0,
    search.statuses.length > 0,
    search.groups.length > 0,
    search.tags.length > 0,
    search.minBalance != null,
    search.maxBalance != null,
    search.minResponseTime != null,
    search.maxResponseTime != null,
    search.start !== reset.start,
    search.end !== reset.end,
  ].filter(Boolean).length
  return (
    <FilterPanel
      advanced={
        <>
          <label className='grid gap-1 text-sm'>
            <span>{t('channelInventory.filters.types')}</span>
            <Input
              inputMode='numeric'
              onChange={(event) =>
                onChange({
                  page: 1,
                  types: commaNumbers(event.target.value, 10_000),
                })
              }
              placeholder={t('channelInventory.filters.typesPlaceholder')}
              value={search.types.join(',')}
            />
          </label>
          <label className='grid gap-1 text-sm'>
            <span>{t('channelInventory.filters.groups')}</span>
            <Input
              onChange={stringList('groups')}
              value={search.groups.join(',')}
            />
          </label>
          <label className='grid gap-1 text-sm'>
            <span>{t('channelInventory.filters.tags')}</span>
            <Input
              onChange={stringList('tags')}
              value={search.tags.join(',')}
            />
          </label>
          <MultiChoice
            label={t('channelInventory.filters.statuses')}
            onChange={(values) =>
              onChange({ page: 1, statuses: values.map(Number) })
            }
            options={statusValues.map((value) => ({
              label: statusText(value, t),
              value,
            }))}
            selected={search.statuses}
          />
          {search.tab === 'list' && (
            <>
              <label className='grid gap-1 text-sm'>
                <span>{t('channelInventory.filters.minBalance')}</span>
                <Input
                  inputMode='decimal'
                  onChange={decimalChange('minBalance')}
                  value={search.minBalance ?? ''}
                />
              </label>
              <label className='grid gap-1 text-sm'>
                <span>{t('channelInventory.filters.maxBalance')}</span>
                <Input
                  inputMode='decimal'
                  onChange={decimalChange('maxBalance')}
                  value={search.maxBalance ?? ''}
                />
              </label>
              <label className='grid gap-1 text-sm'>
                <span>{t('channelInventory.filters.minResponse')}</span>
                <Input
                  inputMode='numeric'
                  onChange={metricChange('minResponseTime')}
                  value={search.minResponseTime ?? ''}
                />
              </label>
              <label className='grid gap-1 text-sm'>
                <span>{t('channelInventory.filters.maxResponse')}</span>
                <Input
                  inputMode='numeric'
                  onChange={metricChange('maxResponseTime')}
                  value={search.maxResponseTime ?? ''}
                />
              </label>
            </>
          )}
          <label className='grid gap-1 text-sm'>
            <span>{t('channelInventory.filters.start')}</span>
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
            <span>{t('channelInventory.filters.end')}</span>
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
      description={t('channelInventory.filters.description')}
      hasActiveFilters={hasFilterChanges(search, reset, [
        'end',
        'groups',
        'keyword',
        'maxBalance',
        'maxResponseTime',
        'minBalance',
        'minResponseTime',
        'siteIds',
        'start',
        'states',
        'statuses',
        'tags',
        'types',
      ])}
      onReset={() => onChange(reset)}
      title={t('channelInventory.filters.title')}
    >
      <div className='flex min-w-0 flex-1 flex-wrap items-center gap-2'>
        {search.tab === 'list' && (
          <label className='grid gap-1 text-sm'>
            <span>{t('channelInventory.filters.keyword')}</span>
            <DebouncedInput
              className='min-w-48 sm:w-72'
              onValueChange={(keyword) => onChange({ keyword, page: 1 })}
              placeholder={t('channelInventory.filters.keywordPlaceholder')}
              value={search.keyword}
            />
          </label>
        )}
        {global && (
          <MultiFacetedFilter
            clearLabel={t('common.all')}
            onChange={(values) =>
              onChange({
                page: 1,
                siteIds: values.filter(isIdString).map(parseIdString),
              })
            }
            options={sites.map((site) => ({
              label: site.name,
              value: site.id,
            }))}
            title={t('channelInventory.filters.siteIds')}
            values={search.siteIds}
          />
        )}
        {search.tab === 'list' && (
          <FacetedFilter
            clearLabel={t('common.all')}
            onChange={(value) =>
              onChange({
                page: 1,
                states: stateValues.includes(value as ChannelInventoryState)
                  ? [value as ChannelInventoryState]
                  : [],
              })
            }
            options={stateValues.map((value) => ({
              label:
                value === 'normal'
                  ? t('channelInventory.state.normal')
                  : t('channelInventory.state.missing'),
              value,
            }))}
            title={t('channelInventory.filters.states')}
            value={search.states.length === 1 ? search.states[0] : ''}
          />
        )}
      </div>
    </FilterPanel>
  )
}

function TrendTable({
  onPageChange,
  onPageSizeChange,
  page,
  pageSize,
  points,
}: {
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
  page: number
  pageSize: number
  points: ChannelInventoryTrendPoint[]
}) {
  const { t } = useTranslation()
  const ordered = useMemo(
    () =>
      [...points].sort((left, right) => right.bucket_start - left.bucket_start),
    [points]
  )
  const offset = (page - 1) * pageSize
  const columns = useMemo<ColumnDef<ChannelInventoryTrendPoint, unknown>[]>(
    () => [
      {
        accessorFn: (point) => timestamp(point.bucket_start),
        header: t('channelInventory.trend.bucket'),
        id: 'bucket',
      },
      {
        accessorFn: (point) => formatMetricDisplayValue(point.channel_count),
        header: t('channelInventory.metric.channelCount'),
      },
      {
        accessorFn: (point) => formatMetricDisplayValue(point.available_count),
        header: t('channelInventory.metric.available'),
      },
      {
        accessorFn: (point) =>
          formatMetricDisplayValue(point.unavailable_count),
        header: t('channelInventory.metric.unavailable'),
      },
      {
        accessorFn: (point) => formatDecimalDisplayValue(point.balance_total),
        header: t('channelInventory.metric.balance'),
      },
      {
        accessorFn: (point) =>
          formatDecimalDisplayValue(point.response_time_avg_ms),
        header: t('channelInventory.metric.responseAvg'),
      },
      {
        accessorFn: (point) => availabilityText(point.availability_rate),
        header: t('channelInventory.metric.availability'),
      },
      {
        cell: ({ row }) => (
          <DataStatusBadge status={row.original.data_status} />
        ),
        header: t('common.status'),
        id: 'status',
      },
    ],
    [t]
  )
  return (
    <DataTable
      ariaLabel={t('channelInventory.trend.table')}
      columns={columns}
      data={ordered.slice(offset, offset + pageSize)}
      emptyTitle={t('channelInventory.trend.empty')}
      onPageChange={onPageChange}
      onPageSizeChange={onPageSizeChange}
      page={page}
      pageSize={pageSize}
      total={ordered.length}
    />
  )
}

function TrendView({
  onSearchChange,
  points,
  search,
}: {
  onSearchChange: (changes: Partial<ChannelInventorySearch>) => void
  points: ChannelInventoryTrendPoint[]
  search: ChannelInventorySearch
}) {
  const { t } = useTranslation()
  const series = useMemo(
    () => [
      {
        key: 'channel_count',
        label: t('channelInventory.metric.channelCount'),
      },
      { key: 'available_count', label: t('channelInventory.metric.available') },
      {
        key: 'unavailable_count',
        label: t('channelInventory.metric.unavailable'),
      },
    ],
    [t]
  )
  return (
    <section className='flex min-h-0 flex-col gap-3 lg:flex-1'>
      <Tabs
        onValueChange={(trendView) =>
          onSearchChange({
            trendPage: 1,
            trendView: trendView as ChannelInventorySearch['trendView'],
          })
        }
        value={search.trendView}
      >
        <TabsList aria-label={t('channelInventory.trend.views.label')}>
          <TabsTrigger value='chart'>
            <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
            {t('channelInventory.trend.views.chart')}
          </TabsTrigger>
          <TabsTrigger value='table'>
            <HugeiconsIcon icon={TableIcon} strokeWidth={2} />
            {t('channelInventory.trend.views.table')}
          </TabsTrigger>
        </TabsList>
      </Tabs>
      {search.trendView === 'chart' ? (
        <InventoryTrendChart
          ariaLabel={t('channelInventory.trend.chart')}
          description={t('channelInventory.trend.chartDescription')}
          emptyText={t('channelInventory.trend.empty')}
          points={points.map((point) => ({
            bucketStart: point.bucket_start,
            dataStatus: point.data_status,
            values: {
              available_count: point.available_count,
              channel_count: point.channel_count,
              unavailable_count: point.unavailable_count,
            },
          }))}
          series={series}
        />
      ) : (
        <TrendTable
          onPageChange={(trendPage) => onSearchChange({ trendPage })}
          onPageSizeChange={(trendPageSize) =>
            onSearchChange({ trendPage: 1, trendPageSize })
          }
          page={search.trendPage}
          pageSize={search.trendPageSize}
          points={points}
        />
      )}
    </section>
  )
}

function BreakdownSection({
  items,
  title,
}: {
  items: ChannelInventoryBreakdown[]
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
              key={`${item.dimension_id}:${item.site_id}`}
            >
              <div className='flex items-start justify-between gap-2'>
                <div>
                  <p className='font-medium'>{item.dimension_name}</p>
                  <code className='text-muted-foreground text-xs'>
                    {item.dimension_id}
                  </code>
                </div>
                <DataStatusBadge status={item.data_status} />
              </div>
              {item.site_name && (
                <p className='text-muted-foreground text-xs'>
                  {item.site_name} · {item.site_id}
                </p>
              )}
              <p className='text-xs'>
                {t('channelInventory.breakdownMetric', {
                  balance: formatDecimalDisplayValue(item.balance_total),
                  channels: formatMetricDisplayValue(item.channel_count),
                  rate: availabilityText(item.availability_rate),
                })}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t('channelInventory.asOf', { time: timestamp(item.as_of) })}
              </p>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

export function ChannelInventoryPage({
  onPageReplace,
  onSearchChange,
  search,
  siteId,
}: {
  onPageReplace: (page: number) => void
  onSearchChange: (changes: Partial<ChannelInventorySearch>) => void
  search: ChannelInventorySearch
  siteId?: string
}) {
  const { t } = useTranslation()
  const [initialJob, setInitialJob] = useState<StatisticsExportJobItem>()
  const validSiteId = siteId == null || isIdString(siteId)
  const currentListParams = useMemo(() => listParams(search), [search])
  const currentStatisticsParams = useMemo(
    () => statisticsParams(search),
    [search]
  )
  const siteParams = useMemo(() => ({}), [])
  const sitesQuery = useQuery({
    enabled: !siteId,
    queryFn: () => listAllSites(siteParams),
    queryKey: siteKeys.options(siteParams),
  })
  const listQuery = useQuery({
    enabled: validSiteId && search.tab === 'list',
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? listSiteChannelInventory(parseIdString(siteId), currentListParams)
        : listChannelInventory(currentListParams),
    queryKey:
      siteId && isIdString(siteId)
        ? channelInventoryKeys.siteList(siteId, currentListParams)
        : channelInventoryKeys.globalList(currentListParams),
  })
  const statisticsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? getSiteChannelInventoryStatistics(
            parseIdString(siteId),
            currentStatisticsParams
          )
        : getChannelInventoryStatistics(currentStatisticsParams),
    queryKey:
      siteId && isIdString(siteId)
        ? channelInventoryKeys.siteStatistics(siteId, currentStatisticsParams)
        : channelInventoryKeys.globalStatistics(currentStatisticsParams),
  })
  const exportMutation = useMutation({
    mutationFn: (format: StatisticsExportFormat) =>
      createStatisticsExport(
        buildChannelInventoryExportRequest(
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
  const retainedScope = siteId ? `site:${siteId}` : 'global'
  const list = useRetainedQueryData(
    listQuery.data,
    listQuery.isError,
    retainedScope
  )
  const statistics = useRetainedQueryData(
    statisticsQuery.data,
    statisticsQuery.isError,
    retainedScope
  )
  useLastValidPage({
    isFetching: listQuery.isFetching,
    isPlaceholderData: listQuery.isPlaceholderData,
    onReplace: onPageReplace,
    page: search.page,
    pageSize: list?.page_size ?? search.pageSize,
    total: list?.total,
  })
  const purpose = purposeText(search.tab, t)
  const columns = useMemo<ColumnDef<ChannelInventoryItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='max-w-40 min-w-32 whitespace-normal'>
            <span className='block font-medium break-words'>
              {row.original.name}
            </span>
            <code className='text-muted-foreground block text-xs break-all'>
              {row.original.remote_channel_id}
            </code>
            <span className='text-muted-foreground block text-xs break-words'>
              {row.original.site_name} · {row.original.site_id}
            </span>
          </div>
        ),
        header: t('channelInventory.identity'),
        id: 'identity',
      },
      {
        cell: ({ row }) => (
          <div className='grid max-w-40 min-w-32 gap-1 text-xs whitespace-normal'>
            <span>
              {t('channelInventory.typeValue', { value: row.original.type })}
            </span>
            <span>{statusText(row.original.status, t)}</span>
            <code className='break-all'>{row.original.group || '-'}</code>
            <code className='break-all'>{row.original.tag || '-'}</code>
          </div>
        ),
        header: t('channelInventory.classification'),
        id: 'classification',
      },
      {
        cell: ({ row }) => (
          <ChannelStateBadge state={row.original.remote_state} />
        ),
        header: t('channelInventory.remoteState'),
        id: 'state',
      },
      {
        cell: ({ row }) => (
          <div className='grid max-w-40 min-w-36 gap-1 text-xs whitespace-normal'>
            <span className='break-all'>
              {t('channelInventory.balanceValue', {
                value: formatDecimalDisplayValue(row.original.balance),
              })}
            </span>
            <span className='break-all'>
              {t('channelInventory.usedQuotaValue', {
                value: formatMetricDisplayValue(row.original.used_quota),
              })}
            </span>
            <span className='break-all'>
              {t('channelInventory.responseValue', {
                value: formatMetricDisplayValue(row.original.response_time_ms),
              })}
            </span>
          </div>
        ),
        header: t('channelInventory.operatingMetrics'),
        id: 'metrics',
      },
      {
        cell: ({ row }) => (
          <div className='grid max-w-36 min-w-32 gap-1 text-xs whitespace-normal'>
            <span>
              {t('channelInventory.priorityValue', {
                value: formatMetricDisplayValue(row.original.priority),
              })}
            </span>
            <span>
              {t('channelInventory.weightValue', {
                value: formatMetricDisplayValue(row.original.weight),
              })}
            </span>
            <span>
              {t('channelInventory.autoBanValue', {
                value: row.original.auto_ban ? t('common.yes') : t('common.no'),
              })}
            </span>
          </div>
        ),
        header: t('channelInventory.scheduling'),
        id: 'scheduling',
      },
      {
        cell: ({ row }) => <ChannelModelList models={row.original.models} />,
        header: t('channelInventory.models'),
        id: 'models',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-40 gap-1 text-xs'>
            <span>
              {t('channelInventory.testTimeValue', {
                time: timestamp(row.original.test_time),
              })}
            </span>
            <span>
              {t('channelInventory.balanceUpdatedValue', {
                time: timestamp(row.original.balance_updated_at),
              })}
            </span>
            <span>
              {t('channelInventory.lastSeenValue', {
                time: timestamp(row.original.last_seen_at),
              })}
            </span>
          </div>
        ),
        header: t('channelInventory.timestamps'),
        id: 'timestamps',
      },
    ],
    [t]
  )
  return (
    <SectionPageLayout
      actions={
        search.tab === 'list'
          ? (['xlsx', 'csv'] as const).map((format) => (
              <Button
                disabled={exportMutation.isPending || !validSiteId}
                key={format}
                onClick={() => exportMutation.mutate(format)}
                variant='outline'
              >
                <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
                {t('channelInventory.export', { format: format.toUpperCase() })}
              </Button>
            ))
          : undefined
      }
      description={
        siteId
          ? t('channelInventory.siteDescription', { id: siteId })
          : t('channelInventory.description')
      }
      fixedContent
      mobileScrollableContent
      title={
        siteId ? t('channelInventory.siteTitle') : t('channelInventory.title')
      }
    >
      <div className='flex min-w-0 flex-col gap-4 lg:h-full lg:min-h-0'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('channelInventory.backToSite')}
          </DetailBackLink>
        )}
        {statistics && <MetricGrid metric={statistics.summary} />}
        <Tabs
          onValueChange={(tab) =>
            onSearchChange(
              changeChannelInventoryTab(tab as ChannelInventorySearch['tab'])
            )
          }
          value={search.tab}
        >
          <TabsList aria-label={t('channelInventory.tabs.label')}>
            <TabsTrigger value='list'>
              {t('channelInventory.tabs.list')}
            </TabsTrigger>
            <TabsTrigger value='trend'>
              {t('channelInventory.tabs.trend')}
            </TabsTrigger>
            <TabsTrigger value='dimensions'>
              {t('channelInventory.tabs.dimensions')}
            </TabsTrigger>
            <TabsTrigger value='sites'>
              {t('channelInventory.tabs.sites')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <section className='border-border bg-muted/20 flex items-start gap-3 rounded-xl border p-4'>
          <span className='bg-background text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg border'>
            <HugeiconsIcon
              icon={search.tab === 'list' ? Database01Icon : Chart01Icon}
              size={18}
              strokeWidth={2}
            />
          </span>
          <div className='min-w-0 flex-1'>
            <div className='flex flex-wrap items-center gap-2'>
              <p className='font-medium'>{purpose.title}</p>
              {statistics && (
                <span className='flex items-center gap-1.5 text-xs'>
                  <span className='text-muted-foreground'>
                    {t('channelInventory.statisticsStatus')}
                  </span>
                  <DataStatusBadge status={statistics.data_status} />
                </span>
              )}
              {search.tab === 'list' && list && (
                <span className='flex items-center gap-1.5 text-xs'>
                  <span className='text-muted-foreground'>
                    {t('channelInventory.listStatus')}
                  </span>
                  <DataStatusBadge status={list.data_status} />
                </span>
              )}
            </div>
            <p className='text-muted-foreground mt-1 text-sm'>
              {purpose.description}
            </p>
            {search.tab === 'list' && list?.as_of != null && (
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('channelInventory.asOf', {
                  time: timestamp(list.as_of),
                })}
              </p>
            )}
            <p className='text-muted-foreground mt-1 flex items-start gap-1.5 text-xs'>
              <HugeiconsIcon
                className='mt-0.5 shrink-0'
                icon={Alert02Icon}
                size={14}
              />
              <span>{t('channelInventory.security.description')}</span>
            </p>
          </div>
        </section>
        <InventoryFilters
          global={!siteId}
          onChange={(changes) => onSearchChange({ ...changes, trendPage: 1 })}
          search={search}
          sites={sitesQuery.data?.items ?? []}
        />
        {!siteId && sitesQuery.isError && (
          <QueryStateAlert
            message={t('common.siteOptionsRefreshFailed')}
            onRetry={() => void sitesQuery.refetch()}
          />
        )}
        {search.tab === 'list' && listQuery.isError && list && (
          <QueryStateAlert
            message={t('common.retainedDataRefreshFailed')}
            onRetry={() => void listQuery.refetch()}
          />
        )}
        {search.tab === 'list' && (
          <DataTable
            ariaLabel={t('channelInventory.table')}
            columns={columns}
            data={list?.items ?? []}
            emptyDescription={t('channelInventory.emptyDescription')}
            emptyTitle={t('channelInventory.empty')}
            error={!validSiteId || (listQuery.isError && !list)}
            fetching={listQuery.isFetching}
            loading={listQuery.isPending}
            mobileCardBreakpoint='wide'
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={validSiteId ? () => void listQuery.refetch() : undefined}
            page={search.page}
            pageSize={search.pageSize}
            renderMobileCard={(item) => (
              <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
                <div className='flex items-start justify-between gap-2'>
                  <div className='min-w-0'>
                    <p className='font-medium break-words'>{item.name}</p>
                    <code className='text-muted-foreground text-xs break-all'>
                      {item.remote_channel_id}
                    </code>
                  </div>
                  <ChannelStateBadge state={item.remote_state} />
                </div>
                <p className='text-muted-foreground text-xs break-words'>
                  {item.site_name} · {item.site_id}
                </p>
                <dl className='grid grid-cols-2 gap-3 text-sm [&>div]:min-w-0'>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('channelInventory.status')}
                    </dt>
                    <dd>{statusText(item.status, t)}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('channelInventory.group')}
                    </dt>
                    <dd className='break-all'>{item.group || '-'}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('channelInventory.metric.balance')}
                    </dt>
                    <dd className='break-all' title={item.balance}>
                      {formatDecimalDisplayValue(item.balance)}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('channelInventory.responseTime')}
                    </dt>
                    <dd title={item.response_time_ms}>
                      {formatMetricDisplayValue(item.response_time_ms)}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('channelInventory.filters.types')}
                    </dt>
                    <dd className='break-all'>{item.type}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('channelInventory.filters.tags')}
                    </dt>
                    <dd className='break-words'>{item.tag || '-'}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('channelInventory.metric.usedQuota')}
                    </dt>
                    <dd className='break-all'>
                      <MetricValue value={item.used_quota} />
                    </dd>
                  </div>
                  <div className='grid gap-0.5 text-xs'>
                    <span>
                      {t('channelInventory.priorityValue', {
                        value: formatMetricDisplayValue(item.priority),
                      })}
                    </span>
                    <span>
                      {t('channelInventory.weightValue', {
                        value: formatMetricDisplayValue(item.weight),
                      })}
                    </span>
                    <span>
                      {t('channelInventory.autoBanValue', {
                        value: item.auto_ban ? t('common.yes') : t('common.no'),
                      })}
                    </span>
                  </div>
                  <div className='col-span-2 grid gap-0.5 text-xs'>
                    <span>
                      {t('channelInventory.testTimeValue', {
                        time: timestamp(item.test_time),
                      })}
                    </span>
                    <span>
                      {t('channelInventory.balanceUpdatedValue', {
                        time: timestamp(item.balance_updated_at),
                      })}
                    </span>
                    <span>
                      {t('channelInventory.lastSeenValue', {
                        time: timestamp(item.last_seen_at),
                      })}
                    </span>
                  </div>
                </dl>
                <div>
                  <p className='text-muted-foreground mb-1 text-xs'>
                    {t('channelInventory.models')}
                  </p>
                  <ChannelModelList models={item.models} />
                </div>
              </article>
            )}
            rowHeaderColumnId='identity'
            total={list?.total ?? 0}
          />
        )}
        {search.tab !== 'list' && statisticsQuery.isPending && !statistics && (
          <LoadingState className='min-h-40' />
        )}
        {statisticsQuery.isError && statistics && (
          <QueryStateAlert
            message={t('common.retainedDataRefreshFailed')}
            onRetry={() => void statisticsQuery.refetch()}
          />
        )}
        {statisticsQuery.isError && !statistics && (
          <ErrorState
            className='min-h-40'
            onRetry={
              validSiteId ? () => void statisticsQuery.refetch() : undefined
            }
            title={t('channelInventory.statisticsError')}
          />
        )}
        {statistics && search.tab !== 'list' && (
          <div
            className='min-h-0 overflow-visible pr-1 lg:flex-1 lg:overflow-y-auto'
            tabIndex={0}
          >
            {search.tab === 'trend' && (
              <TrendView
                onSearchChange={onSearchChange}
                points={statistics.trend}
                search={search}
              />
            )}
            {search.tab === 'dimensions' && (
              <div className='grid gap-6 sm:grid-cols-2 xl:grid-cols-4'>
                <BreakdownSection
                  items={statistics.type_breakdown}
                  title={t('channelInventory.breakdown.type')}
                />
                <BreakdownSection
                  items={statistics.status_breakdown}
                  title={t('channelInventory.breakdown.status')}
                />
                <BreakdownSection
                  items={statistics.group_breakdown}
                  title={t('channelInventory.breakdown.group')}
                />
                <BreakdownSection
                  items={statistics.tag_breakdown}
                  title={t('channelInventory.breakdown.tag')}
                />
              </div>
            )}
            {search.tab === 'sites' && (
              <BreakdownSection
                items={statistics.site_breakdown}
                title={t('channelInventory.breakdown.site')}
              />
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
