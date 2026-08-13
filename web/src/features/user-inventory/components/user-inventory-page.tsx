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
  isIdString,
  isMetricString,
  parseIdString,
  parseMetricString,
} from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'
import { formatDisplayValue } from '@/lib/display-value'
import { hasFilterChanges } from '@/lib/filter-state'

import {
  getSiteUserInventoryStatistics,
  getUserInventoryStatistics,
  listSiteUserInventory,
  listUserInventory,
} from '../api'
import { buildUserInventoryExportRequest } from '../export-request'
import { userInventoryKeys } from '../query-keys'
import {
  buildUserInventorySearch,
  changeUserInventoryTab,
  type UserInventorySearch,
} from '../search'
import type {
  UserInventoryBreakdown,
  UserInventoryItem,
  UserInventoryMetric,
  UserInventoryQueryParams,
  UserInventorySiteBreakdown,
  UserInventoryState,
  UserInventoryStatisticsQueryParams,
  UserInventoryTrendPoint,
} from '../types'

const roles = [0, 1, 10, 100] as const
const statuses = [1, 2] as const
const states: UserInventoryState[] = [
  'normal',
  'missing',
  'deleted',
  'identity_mismatch',
]

function listParams(search: UserInventorySearch): UserInventoryQueryParams {
  return {
    groups: search.groups,
    keyword: search.keyword || undefined,
    max_balance: search.maxBalance,
    min_balance: search.minBalance,
    p: search.page,
    page_size: search.pageSize,
    remote_user_id: search.remoteUserId,
    roles: search.roles,
    site_ids: search.siteIds,
    states: search.states,
    statuses: search.statuses,
  }
}

function statisticsParams(
  search: UserInventorySearch
): UserInventoryStatisticsQueryParams {
  return {
    end_timestamp: search.end,
    groups: search.groups,
    roles: search.roles,
    site_ids: search.siteIds,
    start_timestamp: search.start,
    statuses: search.statuses,
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
  if (!parsed.isValid()) return undefined
  return parsed.startOf('hour').unix()
}

function roleText(role: number, t: (key: string) => string) {
  if (role === 0) return t('userInventory.role.guest')
  if (role === 1) return t('userInventory.role.user')
  if (role === 10) return t('userInventory.role.admin')
  if (role === 100) return t('userInventory.role.root')
  return t('common.unknown')
}

function statusText(status: number, t: (key: string) => string) {
  if (status === 1) return t('userInventory.status.enabled')
  if (status === 2) return t('userInventory.status.disabled')
  return t('common.unknown')
}

function purposeText(
  tab: UserInventorySearch['tab'],
  t: (key: string) => string
) {
  if (tab === 'trend') {
    return {
      description: t('userInventory.purpose.trendDescription'),
      title: t('userInventory.purpose.trendTitle'),
    }
  }
  if (tab === 'dimensions') {
    return {
      description: t('userInventory.purpose.dimensionsDescription'),
      title: t('userInventory.purpose.dimensionsTitle'),
    }
  }
  if (tab === 'sites') {
    return {
      description: t('userInventory.purpose.sitesDescription'),
      title: t('userInventory.purpose.sitesTitle'),
    }
  }
  return {
    description: t('userInventory.purpose.listDescription'),
    title: t('userInventory.purpose.listTitle'),
  }
}

function InventoryStateBadge({ state }: { state: UserInventoryState }) {
  const { t } = useTranslation()
  let variant: 'destructive' | 'neutral' | 'success' | 'warning' = 'success'
  if (state === 'missing') variant = 'warning'
  else if (state === 'deleted') variant = 'neutral'
  else if (state === 'identity_mismatch') variant = 'destructive'
  const labels = {
    deleted: t('userInventory.state.deleted'),
    identity_mismatch: t('userInventory.state.identityMismatch'),
    missing: t('userInventory.state.missing'),
    normal: t('userInventory.state.normal'),
  }
  return <Badge variant={variant}>{labels[state]}</Badge>
}

function MetricGrid({ metric }: { metric: UserInventoryMetric }) {
  const { t } = useTranslation()
  const primary = [
    [Database01Icon, t('userInventory.metric.userCount'), metric.user_count],
    [Chart01Icon, t('userInventory.metric.newUsers'), metric.new_user_count],
    [
      Chart01Icon,
      t('userInventory.metric.activeUsers'),
      metric.active_user_count,
    ],
  ] as const
  const quality = [
    [t('userInventory.metric.quota'), metric.quota],
    [t('userInventory.metric.usedQuota'), metric.used_quota],
    [t('userInventory.metric.balance'), metric.balance],
    [t('userInventory.metric.requestCount'), metric.request_count],
  ] as const
  return (
    <div className='grid gap-3'>
      <div className='grid gap-3 sm:grid-cols-3'>
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
      <dl className='border-border bg-muted/20 grid gap-x-6 gap-y-2 rounded-xl border px-4 py-3 sm:grid-cols-2 xl:grid-cols-4'>
        {quality.map(([label, value]) => (
          <div
            className='flex items-baseline justify-between gap-3'
            key={label}
          >
            <dt className='text-muted-foreground text-xs'>{label}</dt>
            <dd className='font-mono text-sm font-medium'>
              <MetricValue value={value} />
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function MultiChoice({
  label,
  options,
  selected,
  onChange,
}: {
  label: string
  options: ReadonlyArray<{ label: string; value: number | string }>
  selected: readonly (number | string)[]
  onChange: (values: Array<number | string>) => void
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

function InventoryFilters({
  global,
  onChange,
  search,
  sites,
}: {
  global: boolean
  onChange: (changes: Partial<UserInventorySearch>) => void
  search: UserInventorySearch
  sites: SiteListItem[]
}) {
  const { t } = useTranslation()
  const balanceChange =
    (key: 'maxBalance' | 'minBalance') =>
    (event: ChangeEvent<HTMLInputElement>) => {
      const value = event.target.value
      if (value === '') onChange({ [key]: undefined, page: 1 })
      else if (isMetricString(value)) {
        onChange({ [key]: parseMetricString(value), page: 1 })
      }
    }
  const reset = buildUserInventorySearch({
    pageSize: search.pageSize,
    tab: search.tab,
    trendPageSize: search.trendPageSize,
    trendView: search.trendView,
  })
  const advancedCount = [
    search.remoteUserId != null,
    search.groups.length > 0,
    search.roles.length > 0,
    search.statuses.length > 0,
    search.minBalance != null,
    search.maxBalance != null,
    search.start !== reset.start,
    search.end !== reset.end,
  ].filter(Boolean).length
  return (
    <FilterPanel
      advanced={
        <>
          {search.tab === 'list' && (
            <label className='grid gap-1 text-sm'>
              <span>{t('userInventory.filters.remoteUserId')}</span>
              <Input
                inputMode='numeric'
                onChange={(event) => {
                  const value = event.target.value
                  if (value === '') {
                    onChange({ page: 1, remoteUserId: undefined })
                  } else if (isIdString(value)) {
                    onChange({
                      page: 1,
                      remoteUserId: parseIdString(value),
                    })
                  }
                }}
                value={search.remoteUserId ?? ''}
              />
            </label>
          )}
          <label className='grid gap-1 text-sm'>
            <span>{t('userInventory.filters.groups')}</span>
            <Input
              onChange={(event) =>
                onChange({
                  groups: event.target.value
                    .split(',')
                    .map((value) => value.trim())
                    .filter(Boolean),
                  page: 1,
                })
              }
              placeholder={t('userInventory.filters.groupsPlaceholder')}
              value={search.groups.join(',')}
            />
          </label>
          <MultiChoice
            label={t('userInventory.filters.roles')}
            onChange={(values) =>
              onChange({ page: 1, roles: values.map(Number) })
            }
            options={roles.map((value) => ({
              label: roleText(value, t),
              value,
            }))}
            selected={search.roles}
          />
          <MultiChoice
            label={t('userInventory.filters.statuses')}
            onChange={(values) =>
              onChange({ page: 1, statuses: values.map(Number) })
            }
            options={statuses.map((value) => ({
              label: statusText(value, t),
              value,
            }))}
            selected={search.statuses}
          />
          {search.tab === 'list' && (
            <>
              <label className='grid gap-1 text-sm'>
                <span>{t('userInventory.filters.minBalance')}</span>
                <Input
                  inputMode='numeric'
                  onChange={balanceChange('minBalance')}
                  value={search.minBalance ?? ''}
                />
              </label>
              <label className='grid gap-1 text-sm'>
                <span>{t('userInventory.filters.maxBalance')}</span>
                <Input
                  inputMode='numeric'
                  onChange={balanceChange('maxBalance')}
                  value={search.maxBalance ?? ''}
                />
              </label>
            </>
          )}
          <label className='grid gap-1 text-sm'>
            <span>{t('userInventory.filters.start')}</span>
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
            <span>{t('userInventory.filters.end')}</span>
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
      description={t('userInventory.filters.description')}
      hasActiveFilters={hasFilterChanges(search, reset, [
        'end',
        'groups',
        'keyword',
        'maxBalance',
        'minBalance',
        'remoteUserId',
        'roles',
        'siteIds',
        'start',
        'states',
        'statuses',
      ])}
      onReset={() => onChange(reset)}
      title={t('userInventory.filters.title')}
    >
      <div className='flex min-w-0 flex-1 flex-wrap items-center gap-2'>
        {search.tab === 'list' && (
          <label className='grid gap-1 text-sm'>
            <span>{t('userInventory.filters.keyword')}</span>
            <DebouncedInput
              className='min-w-48 sm:w-72'
              onValueChange={(keyword) => onChange({ keyword, page: 1 })}
              placeholder={t('userInventory.filters.keywordPlaceholder')}
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
            title={t('userInventory.filters.siteIds')}
            values={search.siteIds}
          />
        )}
        {search.tab === 'list' && (
          <FacetedFilter
            clearLabel={t('common.all')}
            onChange={(value) =>
              onChange({
                page: 1,
                states: states.includes(value as UserInventoryState)
                  ? [value as UserInventoryState]
                  : [],
              })
            }
            options={states.map((value) => ({
              label: {
                deleted: t('userInventory.state.deleted'),
                identity_mismatch: t('userInventory.state.identityMismatch'),
                missing: t('userInventory.state.missing'),
                normal: t('userInventory.state.normal'),
              }[value],
              value,
            }))}
            title={t('userInventory.filters.states')}
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
  points: UserInventoryTrendPoint[]
}) {
  const { t } = useTranslation()
  const ordered = useMemo(
    () =>
      [...points].sort((left, right) => right.bucket_start - left.bucket_start),
    [points]
  )
  const offset = (page - 1) * pageSize
  const columns = useMemo<ColumnDef<UserInventoryTrendPoint, unknown>[]>(
    () => [
      {
        accessorFn: (point) => timestamp(point.bucket_start),
        header: t('userInventory.trend.bucket'),
        id: 'bucket',
      },
      {
        accessorKey: 'user_count',
        header: t('userInventory.metric.userCount'),
      },
      {
        accessorKey: 'new_user_count',
        header: t('userInventory.metric.newUsers'),
      },
      {
        accessorKey: 'active_user_count',
        header: t('userInventory.metric.activeUsers'),
      },
      { accessorKey: 'balance', header: t('userInventory.metric.balance') },
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
      ariaLabel={t('userInventory.trend.table')}
      columns={columns}
      data={ordered.slice(offset, offset + pageSize)}
      emptyTitle={t('userInventory.trend.empty')}
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
  onSearchChange: (changes: Partial<UserInventorySearch>) => void
  points: UserInventoryTrendPoint[]
  search: UserInventorySearch
}) {
  const { t } = useTranslation()
  const series = useMemo(
    () => [
      { key: 'user_count', label: t('userInventory.metric.userCount') },
      { key: 'new_user_count', label: t('userInventory.metric.newUsers') },
      {
        key: 'active_user_count',
        label: t('userInventory.metric.activeUsers'),
      },
    ],
    [t]
  )
  return (
    <section className='flex min-h-0 flex-1 flex-col gap-3'>
      <Tabs
        onValueChange={(trendView) =>
          onSearchChange({
            trendPage: 1,
            trendView: trendView as UserInventorySearch['trendView'],
          })
        }
        value={search.trendView}
      >
        <TabsList aria-label={t('userInventory.trend.views.label')}>
          <TabsTrigger value='chart'>
            <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
            {t('userInventory.trend.views.chart')}
          </TabsTrigger>
          <TabsTrigger value='table'>
            <HugeiconsIcon icon={TableIcon} strokeWidth={2} />
            {t('userInventory.trend.views.table')}
          </TabsTrigger>
        </TabsList>
      </Tabs>
      {search.trendView === 'chart' ? (
        <InventoryTrendChart
          ariaLabel={t('userInventory.trend.chart')}
          description={t('userInventory.trend.chartDescription')}
          emptyText={t('userInventory.trend.empty')}
          points={points.map((point) => ({
            bucketStart: point.bucket_start,
            dataStatus: point.data_status,
            values: {
              active_user_count: point.active_user_count,
              new_user_count: point.new_user_count,
              user_count: point.user_count,
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
  items: UserInventoryBreakdown[]
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
            <div
              className='border-border grid gap-2 rounded-lg border p-3 sm:grid-cols-[minmax(8rem,1fr)_2fr]'
              key={`${item.dimension_id}:${item.site_id}`}
            >
              <div>
                <p className='font-medium'>{item.dimension_name}</p>
                <code className='text-muted-foreground text-xs'>
                  {item.dimension_id}
                </code>
              </div>
              <div className='grid grid-cols-2 gap-2 text-xs lg:grid-cols-4'>
                <span>
                  {t('userInventory.metric.userValue', {
                    value: item.user_count,
                  })}
                </span>
                <span>
                  {t('userInventory.metric.activeValue', {
                    value: item.active_user_count,
                  })}
                </span>
                <span>
                  {t('userInventory.metric.balanceValue', {
                    value: item.balance,
                  })}
                </span>
                <span>
                  {t('userInventory.metric.requestValue', {
                    value: item.request_count,
                  })}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

function SiteBreakdown({ items }: { items: UserInventorySiteBreakdown[] }) {
  const { t } = useTranslation()
  return (
    <section aria-labelledby='inventory-sites-title' className='grid gap-3'>
      <h2 className='text-lg font-semibold' id='inventory-sites-title'>
        {t('userInventory.siteBreakdown')}
      </h2>
      {items.length === 0 ? (
        <p className='text-muted-foreground text-sm'>{t('common.none')}</p>
      ) : (
        <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-3'>
          {items.map((item) => (
            <article
              className='border-border grid gap-2 rounded-lg border p-4'
              key={item.site_id}
            >
              <div className='flex items-start justify-between gap-2'>
                <div className='min-w-0'>
                  <p className='font-medium break-words'>{item.site_name}</p>
                  <code className='text-muted-foreground text-xs break-all'>
                    {item.site_id}
                  </code>
                </div>
                <DataStatusBadge status={item.data_status} />
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('userInventory.asOf', { time: timestamp(item.as_of) })}
              </p>
              <p className='text-sm break-words'>
                {t('userInventory.siteMetric', {
                  balance: item.balance,
                  users: item.user_count,
                })}
              </p>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

export function UserInventoryPage({
  onPageReplace,
  onSearchChange,
  search,
  siteId,
}: {
  onPageReplace: (page: number) => void
  onSearchChange: (changes: Partial<UserInventorySearch>) => void
  search: UserInventorySearch
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
        ? listSiteUserInventory(parseIdString(siteId), currentListParams)
        : listUserInventory(currentListParams),
    queryKey:
      siteId && isIdString(siteId)
        ? userInventoryKeys.siteList(siteId, currentListParams)
        : userInventoryKeys.globalList(currentListParams),
  })
  const statisticsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? getSiteUserInventoryStatistics(
            parseIdString(siteId),
            currentStatisticsParams
          )
        : getUserInventoryStatistics(currentStatisticsParams),
    queryKey:
      siteId && isIdString(siteId)
        ? userInventoryKeys.siteStatistics(siteId, currentStatisticsParams)
        : userInventoryKeys.globalStatistics(currentStatisticsParams),
  })
  const exportMutation = useMutation({
    mutationFn: (format: StatisticsExportFormat) =>
      createStatisticsExport(
        buildUserInventoryExportRequest(
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
  const columns = useMemo<ColumnDef<UserInventoryItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='max-w-72 min-w-40'>
            <span className='block font-medium break-words'>
              {row.original.username}
            </span>
            <span className='text-muted-foreground block text-xs break-words'>
              {formatDisplayValue(row.original.display_name)}
            </span>
            <code className='text-muted-foreground block text-xs break-all'>
              {row.original.remote_user_id}
            </code>
          </div>
        ),
        header: t('userInventory.userIdentity'),
        id: 'user',
      },
      {
        cell: ({ row }) => (
          <div className='max-w-64 min-w-36'>
            <span className='block break-words'>{row.original.site_name}</span>
            <code className='text-muted-foreground block text-xs break-all'>
              {row.original.site_id}
            </code>
          </div>
        ),
        header: t('userInventory.site'),
        id: 'site',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-28 gap-1 text-xs'>
            <span>{roleText(row.original.role, t)}</span>
            <span>{statusText(row.original.status, t)}</span>
            <code className='break-all'>{row.original.group || '-'}</code>
          </div>
        ),
        header: t('userInventory.roleStatusGroup'),
        id: 'role',
      },
      {
        cell: ({ row }) => (
          <InventoryStateBadge state={row.original.remote_state} />
        ),
        header: t('userInventory.remoteState'),
        id: 'state',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-36 gap-1 text-xs break-all'>
            <span>
              {t('userInventory.metric.quotaValue', {
                value: row.original.quota,
              })}
            </span>
            <span>
              {t('userInventory.metric.usedValue', {
                value: row.original.used_quota,
              })}
            </span>
            <span>
              {t('userInventory.metric.balanceValue', {
                value: row.original.balance,
              })}
            </span>
            <span>
              {t('userInventory.metric.requestValue', {
                value: row.original.request_count,
              })}
            </span>
          </div>
        ),
        header: t('userInventory.metrics'),
        id: 'metrics',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-52 gap-0.5 text-xs'>
            <span>
              {t('userInventory.remoteCreatedAt', {
                time: timestamp(row.original.remote_created_at),
              })}
            </span>
            <span>
              {t('userInventory.firstSeenAt', {
                time: timestamp(row.original.first_seen_at),
              })}
            </span>
            <span>
              {t('userInventory.lastLoginAt', {
                time: timestamp(row.original.last_login_at),
              })}
            </span>
            <span className='text-muted-foreground'>
              {t('userInventory.lastSeen', {
                time: timestamp(row.original.last_seen_at),
              })}
            </span>
            <span className='text-muted-foreground'>
              {t('userInventory.missingCount', {
                value: row.original.missing_count,
              })}
            </span>
          </div>
        ),
        header: t('userInventory.activity'),
        id: 'activity',
      },
      {
        cell: ({ row }) =>
          row.original.account_id ? (
            <Link
              className='text-primary underline-offset-4 hover:underline'
              params={{ accountId: row.original.account_id }}
              to='/accounts/$accountId'
            >
              {t('userInventory.openManagedAccount')}
            </Link>
          ) : (
            <span className='text-muted-foreground text-xs'>
              {t('userInventory.notManaged')}
            </span>
          ),
        header: t('userInventory.managedAccount'),
        id: 'account',
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
                {t('userInventory.export', { format: format.toUpperCase() })}
              </Button>
            ))
          : undefined
      }
      description={
        siteId
          ? t('userInventory.siteDescription', { id: siteId })
          : t('userInventory.description')
      }
      fixedContent
      mobileScrollableContent
      title={siteId ? t('userInventory.siteTitle') : t('userInventory.title')}
    >
      <div className='flex min-w-0 flex-col gap-4 lg:h-full lg:min-h-0'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('userInventory.backToSite')}
          </DetailBackLink>
        )}
        {statistics && <MetricGrid metric={statistics.summary} />}
        <Tabs
          onValueChange={(tab) =>
            onSearchChange(
              changeUserInventoryTab(tab as UserInventorySearch['tab'])
            )
          }
          value={search.tab}
        >
          <TabsList
            aria-label={t('userInventory.tabs.label')}
            className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'
          >
            <TabsTrigger value='list'>
              {t('userInventory.tabs.list')}
            </TabsTrigger>
            <TabsTrigger value='trend'>
              {t('userInventory.tabs.trend')}
            </TabsTrigger>
            <TabsTrigger value='dimensions'>
              {t('userInventory.tabs.dimensions')}
            </TabsTrigger>
            <TabsTrigger value='sites'>
              {t('userInventory.tabs.sites')}
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
                    {t('userInventory.statisticsStatus')}
                  </span>
                  <DataStatusBadge status={statistics.data_status} />
                </span>
              )}
              {search.tab === 'list' && list && (
                <span className='flex items-center gap-1.5 text-xs'>
                  <span className='text-muted-foreground'>
                    {t('userInventory.listStatus')}
                  </span>
                  <DataStatusBadge status={list.data_status} />
                </span>
              )}
            </div>
            <p className='text-muted-foreground mt-1 text-sm'>
              {purpose.description}
            </p>
            <p className='text-muted-foreground mt-1 flex items-start gap-1.5 text-xs'>
              <HugeiconsIcon
                className='mt-0.5 shrink-0'
                icon={Alert02Icon}
                size={14}
              />
              <span>{t('userInventory.boundary.description')}</span>
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
            ariaLabel={t('userInventory.table')}
            columns={columns}
            data={list?.items ?? []}
            emptyDescription={t('userInventory.emptyDescription')}
            emptyTitle={t('userInventory.empty')}
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
                    <p className='font-medium break-words'>{item.username}</p>
                    <p className='text-muted-foreground text-xs break-words'>
                      {formatDisplayValue(item.display_name)}
                    </p>
                    <code className='text-muted-foreground text-xs break-all'>
                      {item.remote_user_id}
                    </code>
                  </div>
                  <InventoryStateBadge state={item.remote_state} />
                </div>
                <p className='text-muted-foreground text-xs break-words'>
                  {item.site_name} · {item.site_id}
                </p>
                <dl className='grid grid-cols-2 gap-3 text-sm'>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('userInventory.role')}
                    </dt>
                    <dd>{roleText(item.role, t)}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('common.status')}
                    </dt>
                    <dd>{statusText(item.status, t)}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('userInventory.group')}
                    </dt>
                    <dd className='break-words'>{item.group || '-'}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('userInventory.metric.balance')}
                    </dt>
                    <dd className='break-all'>{item.balance}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('userInventory.metric.requestCount')}
                    </dt>
                    <dd className='break-all'>{item.request_count}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('userInventory.metric.quota')}
                    </dt>
                    <dd className='break-all'>{item.quota}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('userInventory.metric.usedQuota')}
                    </dt>
                    <dd className='break-all'>{item.used_quota}</dd>
                  </div>
                  <div className='col-span-2'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('userInventory.activity')}
                    </dt>
                    <dd className='mt-1 grid gap-x-3 gap-y-1 text-xs sm:grid-cols-2'>
                      <span>
                        {t('userInventory.remoteCreatedAt', {
                          time: timestamp(item.remote_created_at),
                        })}
                      </span>
                      <span>
                        {t('userInventory.firstSeenAt', {
                          time: timestamp(item.first_seen_at),
                        })}
                      </span>
                      <span>
                        {t('userInventory.lastLoginAt', {
                          time: timestamp(item.last_login_at),
                        })}
                      </span>
                      <span>
                        {t('userInventory.lastSeen', {
                          time: timestamp(item.last_seen_at),
                        })}
                      </span>
                      <span>
                        {t('userInventory.missingCount', {
                          value: item.missing_count,
                        })}
                      </span>
                    </dd>
                  </div>
                </dl>
                {item.account_id ? (
                  <Link
                    className='text-primary text-sm underline-offset-4 hover:underline'
                    params={{ accountId: item.account_id }}
                    to='/accounts/$accountId'
                  >
                    {t('userInventory.openManagedAccount')}
                  </Link>
                ) : (
                  <span className='text-muted-foreground text-xs'>
                    {t('userInventory.notManaged')}
                  </span>
                )}
              </article>
            )}
            rowHeaderColumnId='user'
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
            title={t('userInventory.statisticsError')}
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
              <div className='grid gap-6 xl:grid-cols-3'>
                <BreakdownSection
                  items={statistics.role_breakdown}
                  title={t('userInventory.breakdown.role')}
                />
                <BreakdownSection
                  items={statistics.status_breakdown}
                  title={t('userInventory.breakdown.status')}
                />
                <BreakdownSection
                  items={statistics.group_breakdown}
                  title={t('userInventory.breakdown.group')}
                />
              </div>
            )}
            {search.tab === 'sites' && (
              <SiteBreakdown items={statistics.site_breakdown} />
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
