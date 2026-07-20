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
  isDecimalString,
  isIdString,
  isMetricString,
  parseDecimalString,
  parseIdString,
  parseMetricString,
} from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'

import {
  getChannelInventoryStatistics,
  getSiteChannelInventoryStatistics,
  listChannelInventory,
  listSiteChannelInventory,
} from '../api'
import { buildChannelInventoryExportRequest } from '../export-request'
import { channelInventoryKeys } from '../query-keys'
import type { ChannelInventorySearch } from '../search'
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
  const values = [
    [t('channelInventory.metric.channelCount'), metric.channel_count],
    [t('channelInventory.metric.available'), metric.available_count],
    [t('channelInventory.metric.unavailable'), metric.unavailable_count],
    [t('channelInventory.metric.missing'), metric.missing_count],
    [t('channelInventory.metric.balance'), metric.balance_total],
    [t('channelInventory.metric.usedQuota'), metric.used_quota],
    [t('channelInventory.metric.responseAvg'), metric.response_time_avg_ms],
    [t('channelInventory.metric.responseMax'), metric.response_time_max_ms],
    [t('channelInventory.metric.availability'), metric.availability_rate],
  ] as const
  return (
    <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 xl:grid-cols-5'>
      {values.map(([label, value]) => (
        <div
          className='border-border min-w-0 border-b p-3 xl:border-r'
          key={label}
        >
          <dt className='text-muted-foreground text-xs'>{label}</dt>
          <dd className='mt-1 text-lg font-semibold break-all'>
            <MetricValue value={value} />
          </dd>
        </div>
      ))}
    </dl>
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
}: {
  global: boolean
  onChange: (changes: Partial<ChannelInventorySearch>) => void
  search: ChannelInventorySearch
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
  return (
    <section
      aria-labelledby='channel-inventory-filters-title'
      className='border-border bg-card grid gap-4 rounded-lg border p-4'
    >
      <div>
        <h2 className='font-medium' id='channel-inventory-filters-title'>
          {t('channelInventory.filters.title')}
        </h2>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t('channelInventory.filters.description')}
        </p>
      </div>
      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <label className='grid gap-1 text-sm'>
          <span>{t('channelInventory.filters.keyword')}</span>
          <Input
            onChange={(event) =>
              onChange({ keyword: event.target.value, page: 1 })
            }
            placeholder={t('channelInventory.filters.keywordPlaceholder')}
            value={search.keyword}
          />
        </label>
        {global && (
          <label className='grid gap-1 text-sm'>
            <span>{t('channelInventory.filters.siteIds')}</span>
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
              placeholder={t('channelInventory.filters.siteIdsPlaceholder')}
              value={search.siteIds.join(',')}
            />
          </label>
        )}
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
          <Input onChange={stringList('tags')} value={search.tags.join(',')} />
        </label>
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
        <label className='grid gap-1 text-sm'>
          <span>{t('channelInventory.filters.start')}</span>
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
          <span>{t('channelInventory.filters.end')}</span>
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
      <div className='grid gap-3 sm:grid-cols-2'>
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
        <MultiChoice
          label={t('channelInventory.filters.states')}
          onChange={(values) =>
            onChange({ page: 1, states: values as ChannelInventoryState[] })
          }
          options={stateValues.map((value) => ({
            label:
              value === 'normal'
                ? t('channelInventory.state.normal')
                : t('channelInventory.state.missing'),
            value,
          }))}
          selected={search.states}
        />
      </div>
    </section>
  )
}

function TrendTable({ points }: { points: ChannelInventoryTrendPoint[] }) {
  const { t } = useTranslation()
  return (
    <section
      aria-labelledby='channel-inventory-trend-title'
      className='grid gap-3'
    >
      <h2 className='text-lg font-semibold' id='channel-inventory-trend-title'>
        {t('channelInventory.trend.title')}
      </h2>
      {points.length === 0 ? (
        <p className='text-muted-foreground text-sm'>
          {t('channelInventory.trend.empty')}
        </p>
      ) : (
        <div className='overflow-x-auto rounded-lg border'>
          <table className='w-full min-w-4xl text-sm'>
            <thead className='bg-muted/70 text-left'>
              <tr>
                <th className='px-3 py-2'>
                  {t('channelInventory.trend.bucket')}
                </th>
                <th className='px-3 py-2'>
                  {t('channelInventory.metric.channelCount')}
                </th>
                <th className='px-3 py-2'>
                  {t('channelInventory.metric.available')}
                </th>
                <th className='px-3 py-2'>
                  {t('channelInventory.metric.balance')}
                </th>
                <th className='px-3 py-2'>
                  {t('channelInventory.metric.responseAvg')}
                </th>
                <th className='px-3 py-2'>
                  {t('channelInventory.metric.availability')}
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
                  <td className='px-3 py-2'>{point.channel_count}</td>
                  <td className='px-3 py-2'>{point.available_count}</td>
                  <td className='px-3 py-2'>{point.balance_total}</td>
                  <td className='px-3 py-2'>{point.response_time_avg_ms}</td>
                  <td className='px-3 py-2'>{point.availability_rate}</td>
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
                  balance: item.balance_total,
                  channels: item.channel_count,
                  rate: item.availability_rate,
                })}
              </p>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

export function ChannelInventoryPage({
  onSearchChange,
  search,
  siteId,
}: {
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
  const listQuery = useQuery({
    enabled: validSiteId,
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
  const list = listQuery.data
  const statistics = statisticsQuery.data
  const columns = useMemo<ColumnDef<ChannelInventoryItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <div className='min-w-40'>
            <span className='font-medium'>{row.original.name}</span>
            <code className='text-muted-foreground block text-xs'>
              {row.original.remote_channel_id}
            </code>
            <span className='text-muted-foreground block text-xs'>
              {row.original.site_name} · {row.original.site_id}
            </span>
          </div>
        ),
        header: t('channelInventory.identity'),
        id: 'identity',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-36 gap-1 text-xs'>
            <span>
              {t('channelInventory.typeValue', { value: row.original.type })}
            </span>
            <span>{statusText(row.original.status, t)}</span>
            <code>{row.original.group || '-'}</code>
            <code>{row.original.tag || '-'}</code>
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
          <div className='grid min-w-40 gap-1 text-xs'>
            <span>
              {t('channelInventory.balanceValue', {
                value: row.original.balance,
              })}
            </span>
            <span>
              {t('channelInventory.usedQuotaValue', {
                value: row.original.used_quota,
              })}
            </span>
            <span>
              {t('channelInventory.responseValue', {
                value: row.original.response_time_ms,
              })}
            </span>
          </div>
        ),
        header: t('channelInventory.operatingMetrics'),
        id: 'metrics',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-36 gap-1 text-xs'>
            <span>
              {t('channelInventory.priorityValue', {
                value: row.original.priority,
              })}
            </span>
            <span>
              {t('channelInventory.weightValue', {
                value: row.original.weight,
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
        cell: ({ row }) => (
          <p className='max-w-64 break-words'>{row.original.models || '-'}</p>
        ),
        header: t('channelInventory.models'),
        id: 'models',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-44 gap-1 text-xs'>
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
        <>
          {(['xlsx', 'csv'] as const).map((format) => (
            <Button
              disabled={exportMutation.isPending || !validSiteId}
              key={format}
              onClick={() => exportMutation.mutate(format)}
              variant='outline'
            >
              <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
              {t('channelInventory.export', { format: format.toUpperCase() })}
            </Button>
          ))}
        </>
      }
      description={
        siteId
          ? t('channelInventory.siteDescription', { id: siteId })
          : t('channelInventory.description')
      }
      title={
        siteId ? t('channelInventory.siteTitle') : t('channelInventory.title')
      }
    >
      <div className='grid min-w-0 gap-6'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('channelInventory.backToSite')}
          </DetailBackLink>
        )}
        <section
          className='border-destructive/20 bg-muted/30 rounded-lg border p-4'
          role='note'
        >
          <p className='font-medium'>{t('channelInventory.security.title')}</p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('channelInventory.security.description')}
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
              className='border-border flex items-center justify-between gap-2 rounded-lg border p-3'
              role='status'
            >
              <div className='flex items-center gap-2'>
                <span className='text-sm font-medium'>
                  {t('channelInventory.listStatus')}
                </span>
                <DataStatusBadge status={list.data_status} />
              </div>
              <span className='text-muted-foreground text-xs'>
                {t('channelInventory.asOf', { time: timestamp(list.as_of) })}
              </span>
            </section>
          )}
          {statistics && (
            <section
              className='border-border flex items-center gap-2 rounded-lg border p-3'
              role='status'
            >
              <span className='text-sm font-medium'>
                {t('channelInventory.statisticsStatus')}
              </span>
              <DataStatusBadge status={statistics.data_status} />
            </section>
          )}
        </div>
        {statistics && <MetricGrid metric={statistics.summary} />}
        <DataTable
          ariaLabel={t('channelInventory.table')}
          columns={columns}
          data={list?.items ?? []}
          emptyDescription={t('channelInventory.emptyDescription')}
          emptyTitle={t('channelInventory.empty')}
          error={!validSiteId || listQuery.isError}
          fetching={listQuery.isFetching}
          loading={listQuery.isPending}
          onPageChange={(page) => onSearchChange({ page })}
          onRetry={() => void listQuery.refetch()}
          page={search.page}
          pageSize={search.pageSize}
          renderMobileCard={(item) => (
            <article className='border-border bg-card grid gap-3 rounded-lg border p-4'>
              <div className='flex items-start justify-between gap-2'>
                <div>
                  <p className='font-medium'>{item.name}</p>
                  <code className='text-muted-foreground text-xs'>
                    {item.remote_channel_id}
                  </code>
                </div>
                <ChannelStateBadge state={item.remote_state} />
              </div>
              <p className='text-muted-foreground text-xs'>
                {item.site_name} · {item.site_id}
              </p>
              <dl className='grid grid-cols-2 gap-3 text-sm'>
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
                  <dd>{item.group || '-'}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('channelInventory.metric.balance')}
                  </dt>
                  <dd className='break-all'>{item.balance}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('channelInventory.responseTime')}
                  </dt>
                  <dd>{item.response_time_ms}</dd>
                </div>
              </dl>
              <p className='text-xs break-words'>{item.models || '-'}</p>
            </article>
          )}
          total={list?.total ?? 0}
        />
        {statisticsQuery.isError && !statistics && (
          <section className='border-destructive/30 bg-destructive/5 rounded-lg border p-4'>
            <p className='text-sm'>{t('channelInventory.statisticsError')}</p>
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
            <BreakdownSection
              items={statistics.site_breakdown}
              title={t('channelInventory.breakdown.site')}
            />
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
