import {
  Alert02Icon,
  ArrowLeft01Icon,
  Chart01Icon,
  Database01Icon,
  FileExportIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataStatusBadge } from '@/components/data/data-status'
import { FacetedFilter } from '@/components/data/faceted-filter'
import { FilterPanel } from '@/components/data/filter-panel'
import { MetricValue } from '@/components/data/metric-value'
import { ErrorState } from '@/components/error-state'
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
            <Input
              className='min-w-48 sm:w-72'
              onChange={(event) =>
                onChange({ keyword: event.target.value, page: 1 })
              }
              placeholder={t('userInventory.filters.keywordPlaceholder')}
              value={search.keyword}
            />
          </label>
        )}
        {global && (
          <FacetedFilter
            clearLabel={t('common.all')}
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
            title={t('userInventory.filters.siteIds')}
            value={search.siteIds.length === 1 ? search.siteIds[0] : ''}
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

function TrendTable({ points }: { points: UserInventoryTrendPoint[] }) {
  const { t } = useTranslation()
  return (
    <section aria-labelledby='inventory-trend-title' className='grid gap-3'>
      <h2 className='text-lg font-semibold' id='inventory-trend-title'>
        {t('userInventory.trend.title')}
      </h2>
      {points.length === 0 ? (
        <p className='text-muted-foreground text-sm'>
          {t('userInventory.trend.empty')}
        </p>
      ) : (
        <div className='overflow-x-auto rounded-lg border'>
          <table className='w-full min-w-3xl text-sm'>
            <thead className='bg-[var(--table-header)] text-left'>
              <tr>
                <th className='px-3 py-2'>{t('userInventory.trend.bucket')}</th>
                <th className='px-3 py-2'>
                  {t('userInventory.metric.userCount')}
                </th>
                <th className='px-3 py-2'>
                  {t('userInventory.metric.newUsers')}
                </th>
                <th className='px-3 py-2'>
                  {t('userInventory.metric.activeUsers')}
                </th>
                <th className='px-3 py-2'>
                  {t('userInventory.metric.balance')}
                </th>
                <th className='px-3 py-2'>{t('common.status')}</th>
              </tr>
            </thead>
            <tbody>
              {points.map((point) => (
                <tr
                  className='border-t transition-colors hover:bg-[var(--table-header-hover)]'
                  key={point.bucket_start}
                >
                  <td className='px-3 py-2 whitespace-nowrap'>
                    {timestamp(point.bucket_start)}
                  </td>
                  <td className='px-3 py-2'>{point.user_count}</td>
                  <td className='px-3 py-2'>{point.new_user_count}</td>
                  <td className='px-3 py-2'>{point.active_user_count}</td>
                  <td className='px-3 py-2'>{point.balance}</td>
                  <td className='px-3 py-2'>
                    <DataStatusBadge status={point.data_status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
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
      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-3'>
        {items.map((item) => (
          <article
            className='border-border grid gap-2 rounded-lg border p-4'
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
              {t('userInventory.asOf', { time: timestamp(item.as_of) })}
            </p>
            <p className='text-sm'>
              {t('userInventory.siteMetric', {
                balance: item.balance,
                users: item.user_count,
              })}
            </p>
          </article>
        ))}
      </div>
    </section>
  )
}

export function UserInventoryPage({
  onSearchChange,
  search,
  siteId,
}: {
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
  const siteParams = useMemo(() => ({ page_size: 100 }), [])
  const sitesQuery = useQuery({
    enabled: !siteId,
    queryFn: () => listSites(siteParams),
    queryKey: siteKeys.list(siteParams),
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
  const list = listQuery.data
  const statistics = statisticsQuery.data
  const purpose = purposeText(search.tab, t)
  const columns = useMemo<ColumnDef<UserInventoryItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='min-w-40'>
            <span className='font-medium'>{row.original.username}</span>
            <span className='text-muted-foreground block text-xs'>
              {formatDisplayValue(row.original.display_name)}
            </span>
            <code className='text-muted-foreground block text-xs'>
              {row.original.remote_user_id}
            </code>
          </div>
        ),
        header: t('userInventory.userIdentity'),
        id: 'user',
      },
      {
        cell: ({ row }) => (
          <div className='min-w-36'>
            <span>{row.original.site_name}</span>
            <code className='text-muted-foreground block text-xs'>
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
            <code>{row.original.group || '-'}</code>
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
          <div className='grid min-w-36 gap-1 text-xs'>
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
          <div className='min-w-40 text-xs'>
            <span>{timestamp(row.original.last_login_at)}</span>
            <span className='text-muted-foreground block'>
              {t('userInventory.lastSeen', {
                time: timestamp(row.original.last_seen_at),
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
      title={siteId ? t('userInventory.siteTitle') : t('userInventory.title')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
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
          <TabsList aria-label={t('userInventory.tabs.label')}>
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
                <DataStatusBadge status={statistics.data_status} />
              )}
              {search.tab === 'list' && list && (
                <DataStatusBadge status={list.data_status} />
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
          onChange={onSearchChange}
          search={search}
          sites={sitesQuery.data?.items ?? []}
        />
        {search.tab === 'list' && (
          <DataTable
            ariaLabel={t('userInventory.table')}
            columns={columns}
            data={list?.items ?? []}
            emptyDescription={t('userInventory.emptyDescription')}
            emptyTitle={t('userInventory.empty')}
            error={!validSiteId || listQuery.isError}
            fetching={listQuery.isFetching}
            loading={listQuery.isPending}
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
                  <div>
                    <p className='font-medium'>{item.username}</p>
                    <code className='text-muted-foreground text-xs'>
                      {item.remote_user_id}
                    </code>
                  </div>
                  <InventoryStateBadge state={item.remote_state} />
                </div>
                <p className='text-muted-foreground text-xs'>
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
                      {t('userInventory.group')}
                    </dt>
                    <dd>{item.group || '-'}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('userInventory.metric.balance')}
                    </dt>
                    <dd>{item.balance}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('userInventory.metric.requestCount')}
                    </dt>
                    <dd>{item.request_count}</dd>
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
            total={list?.total ?? 0}
          />
        )}
        {statisticsQuery.isError && !statistics && (
          <ErrorState
            className='min-h-40'
            onRetry={() => void statisticsQuery.refetch()}
            title={t('userInventory.statisticsError')}
          />
        )}
        {statistics && search.tab !== 'list' && (
          <div className='min-h-0 flex-1 overflow-y-auto pr-1' tabIndex={0}>
            {search.tab === 'trend' && <TrendTable points={statistics.trend} />}
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
