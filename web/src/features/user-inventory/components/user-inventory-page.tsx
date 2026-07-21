import { ArrowLeft01Icon, FileExportIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState, type ChangeEvent } from 'react'
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
  isMetricString,
  parseIdString,
  parseMetricString,
} from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'

import {
  getSiteUserInventoryStatistics,
  getUserInventoryStatistics,
  listSiteUserInventory,
  listUserInventory,
} from '../api'
import { buildUserInventoryExportRequest } from '../export-request'
import { userInventoryKeys } from '../query-keys'
import type { UserInventorySearch } from '../search'
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
  const values = [
    [t('userInventory.metric.userCount'), metric.user_count],
    [t('userInventory.metric.newUsers'), metric.new_user_count],
    [t('userInventory.metric.activeUsers'), metric.active_user_count],
    [t('userInventory.metric.quota'), metric.quota],
    [t('userInventory.metric.usedQuota'), metric.used_quota],
    [t('userInventory.metric.balance'), metric.balance],
    [t('userInventory.metric.requestCount'), metric.request_count],
  ] as const
  return (
    <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 xl:grid-cols-7'>
      {values.map(([label, value]) => (
        <div
          className='border-border min-w-0 border-b p-3 xl:border-r'
          key={label}
        >
          <dt className='text-muted-foreground text-xs'>{label}</dt>
          <dd className='mt-1 text-lg font-semibold'>
            <MetricValue value={value} />
          </dd>
        </div>
      ))}
    </dl>
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
}: {
  global: boolean
  onChange: (changes: Partial<UserInventorySearch>) => void
  search: UserInventorySearch
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
  return (
    <section
      aria-labelledby='user-inventory-filters-title'
      className='border-border bg-card grid gap-4 rounded-lg border p-4'
    >
      <div>
        <h2 className='font-medium' id='user-inventory-filters-title'>
          {t('userInventory.filters.title')}
        </h2>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t('userInventory.filters.description')}
        </p>
      </div>
      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <label className='grid gap-1 text-sm'>
          <span>{t('userInventory.filters.keyword')}</span>
          <Input
            onChange={(event) =>
              onChange({ keyword: event.target.value, page: 1 })
            }
            placeholder={t('userInventory.filters.keywordPlaceholder')}
            value={search.keyword}
          />
        </label>
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
        {global && (
          <label className='grid gap-1 text-sm'>
            <span>{t('userInventory.filters.siteIds')}</span>
            <Input
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
              placeholder={t('userInventory.filters.siteIdsPlaceholder')}
              value={search.siteIds.join(',')}
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
        <label className='grid gap-1 text-sm'>
          <span>{t('userInventory.filters.start')}</span>
          <Input
            onChange={(event) => {
              const start = parseDateTime(event.target.value)
              if (start != null) onChange({ start })
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
              if (end != null) onChange({ end })
            }}
            type='datetime-local'
            value={dateTimeValue(search.end)}
          />
        </label>
      </div>
      <div className='grid gap-3 xl:grid-cols-3'>
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
        <MultiChoice
          label={t('userInventory.filters.states')}
          onChange={(values) =>
            onChange({
              page: 1,
              states: values as UserInventoryState[],
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
          selected={search.states}
        />
      </div>
    </section>
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
            <thead className='bg-muted/70 text-left'>
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
                <tr className='border-t' key={point.bucket_start}>
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
  const listQuery = useQuery({
    enabled: validSiteId,
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
  const columns = useMemo<ColumnDef<UserInventoryItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='min-w-40'>
            <span className='font-medium'>{row.original.username}</span>
            <span className='text-muted-foreground block text-xs'>
              {row.original.display_name || t('common.none')}
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
              className='text-primary-strong underline-offset-4 hover:underline'
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
        <>
          {(['xlsx', 'csv'] as const).map((format) => (
            <Button
              disabled={exportMutation.isPending || !validSiteId}
              key={format}
              onClick={() => exportMutation.mutate(format)}
              variant='outline'
            >
              <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
              {t('userInventory.export', { format: format.toUpperCase() })}
            </Button>
          ))}
        </>
      }
      description={
        siteId
          ? t('userInventory.siteDescription', { id: siteId })
          : t('userInventory.description')
      }
      title={siteId ? t('userInventory.siteTitle') : t('userInventory.title')}
    >
      <div className='grid min-w-0 gap-6'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('userInventory.backToSite')}
          </DetailBackLink>
        )}
        <section
          className='border-primary/30 bg-primary/5 rounded-lg border p-4'
          role='note'
        >
          <p className='font-medium'>{t('userInventory.boundary.title')}</p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('userInventory.boundary.description')}
          </p>
        </section>
        <InventoryFilters
          global={!siteId}
          onChange={onSearchChange}
          search={search}
        />
        <div className='grid gap-3 sm:grid-cols-2'>
          {list && (
            <section
              className='border-border flex items-center gap-2 rounded-lg border p-3'
              role='status'
            >
              <span className='text-sm font-medium'>
                {t('userInventory.listStatus')}
              </span>
              <DataStatusBadge status={list.data_status} />
            </section>
          )}
          {statistics && (
            <section
              className='border-border flex items-center gap-2 rounded-lg border p-3'
              role='status'
            >
              <span className='text-sm font-medium'>
                {t('userInventory.statisticsStatus')}
              </span>
              <DataStatusBadge status={statistics.data_status} />
            </section>
          )}
        </div>
        {statistics && <MetricGrid metric={statistics.summary} />}
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
          onPageSizeChange={(pageSize) => onSearchChange({ page: 1, pageSize })}
          onRetry={() => void listQuery.refetch()}
          page={search.page}
          pageSize={search.pageSize}
          renderMobileCard={(item) => (
            <article className='border-border bg-card grid gap-3 rounded-lg border p-4'>
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
                  className='text-primary-strong text-sm underline-offset-4 hover:underline'
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
        {statisticsQuery.isError && !statistics && (
          <section className='border-destructive/30 bg-destructive/5 rounded-lg border p-4'>
            <p className='text-sm'>{t('userInventory.statisticsError')}</p>
            <Button
              className='mt-3'
              onClick={() => void statisticsQuery.refetch()}
              variant='outline'
            >
              {t('common.retry')}
            </Button>
          </section>
        )}
        {statistics && (
          <>
            <TrendTable points={statistics.trend} />
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
            <SiteBreakdown items={statistics.site_breakdown} />
          </>
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
