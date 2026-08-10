import {
  ArrowLeft01Icon,
  Calendar03Icon,
  Chart01Icon,
  Clock01Icon,
  Database01Icon,
  FileExportIcon,
  Search01Icon,
  Settings02Icon,
  ViewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { type ReactNode, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataStatusBadge } from '@/components/data/data-status'
import { FacetedFilter } from '@/components/data/faceted-filter'
import { QueryStateAlert } from '@/components/data/query-state-alert'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { SelectControl } from '@/components/ui/select-control'
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
import {
  calculateCrossSiteQuotaAmount,
  calculateQuotaAmount,
  formatDecimal,
  PRECISE_AMOUNT_FRACTION_DIGITS,
} from '@/lib/amount'
import { getApiErrorTranslationKey } from '@/lib/api'
import {
  isIdString,
  isNonNegativeIdString,
  parseIdString,
  parseNonNegativeIdString,
  type MetricString,
  type RateInfo,
} from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'
import { formatDisplayValue } from '@/lib/display-value'
import { hasFilterChanges } from '@/lib/filter-state'
import { cn } from '@/lib/utils'

import { getLogStats, getSiteLogStats, listLogs, listSiteLogs } from '../api'
import { isConsumptionLogType } from '../display'
import { buildLogExportRequest } from '../export-request'
import { logKeys } from '../query-keys'
import {
  buildLogQuickRange,
  buildLogSearch,
  getLogQuickRange,
  logQuickRanges,
  type LogQuickRange,
} from '../search'
import type {
  LogDataStatus,
  LogItem,
  LogQueryParams,
  LogSearch,
  LogType,
} from '../types'

const logTypes: LogType[] = [0, 1, 2, 3, 4, 5, 6, 7]

function logParams(search: LogSearch): LogQueryParams {
  return {
    channel_id: search.channelId,
    end_timestamp: search.end,
    group: search.group || undefined,
    model_name: search.modelName || undefined,
    p: search.page,
    page_size: search.pageSize,
    request_id: search.requestId || undefined,
    site_ids: search.siteIds,
    start_timestamp: search.start,
    token_name: search.tokenName || undefined,
    type: search.type,
    upstream_request_id: search.upstreamRequestId || undefined,
    username: search.username || undefined,
  }
}

function dateTimeValue(timestamp: number) {
  return fromUnixSeconds(timestamp).format('YYYY-MM-DDTHH:mm')
}

function parseDateTime(value: string): number | undefined {
  const parsed = dayjs.tz(value, 'YYYY-MM-DDTHH:mm', BEIJING_TIMEZONE)
  return parsed.isValid() ? parsed.unix() : undefined
}

function LogTypeBadge({ type }: { type: LogType }) {
  const { t } = useTranslation()
  let variant: 'destructive' | 'neutral' | 'success' = 'neutral'
  if (type === 5) variant = 'destructive'
  else if (type === 2) variant = 'success'
  const labels = {
    0: t('logs.type.0'),
    1: t('logs.type.1'),
    2: t('logs.type.2'),
    3: t('logs.type.3'),
    4: t('logs.type.4'),
    5: t('logs.type.5'),
    6: t('logs.type.6'),
    7: t('logs.type.7'),
  } as const
  return <Badge variant={variant}>{labels[type]}</Badge>
}

function statusDescription(status: LogDataStatus, t: (key: string) => string) {
  switch (status) {
    case 'complete':
      return t('logs.status.complete')
    case 'partial':
      return t('logs.status.partial')
    case 'pending':
      return t('logs.status.pending')
    case 'unavailable':
      return t('logs.status.unavailable')
    case 'disabled':
      return t('logs.status.disabled')
    case 'missing':
      return t('logs.status.missing')
    case 'paused':
      return t('logs.status.paused')
    case 'backfilling':
      return t('logs.status.backfilling')
  }
}

function logTypeLabel(type: LogType, t: (key: string) => string) {
  switch (type) {
    case 0:
      return t('logs.type.0')
    case 1:
      return t('logs.type.1')
    case 2:
      return t('logs.type.2')
    case 3:
      return t('logs.type.3')
    case 4:
      return t('logs.type.4')
    case 5:
      return t('logs.type.5')
    case 6:
      return t('logs.type.6')
    case 7:
      return t('logs.type.7')
  }
}

type LogTranslate = (
  key: string,
  options?: Record<string, string | number>
) => string

function formatMilliseconds(value: string | null) {
  if (value == null) return '-'
  try {
    const milliseconds = BigInt(value)
    if (milliseconds < 0n) return '-'
    const tenths = (milliseconds + 50n) / 100n
    return formatDurationTenths(tenths)
  } catch {
    return '-'
  }
}

function formatDurationSeconds(value: string) {
  try {
    const seconds = BigInt(value)
    if (seconds < 0n) return '-'
    return formatDurationTenths(seconds * 10n)
  } catch {
    return '-'
  }
}

function formatDurationTenths(tenths: bigint) {
  if (tenths < 600n) {
    return `${tenths / 10n}.${tenths % 10n}s`
  }
  const roundedSeconds = (tenths + 5n) / 10n
  return `${roundedSeconds / 60n}m ${roundedSeconds % 60n}s`
}

function cacheWriteTokens(item: LogItem) {
  const fiveMinutes = tokenBigInt(item.cache_creation_tokens_5m)
  const oneHour = tokenBigInt(item.cache_creation_tokens_1h)
  if (fiveMinutes > 0n || oneHour > 0n) return fiveMinutes + oneHour
  return tokenBigInt(item.cache_creation_tokens)
}

function tokenBigInt(value: string | bigint) {
  try {
    const parsed = BigInt(value)
    return parsed >= 0n ? parsed : 0n
  } catch {
    return 0n
  }
}

function formatTokenCount(value: string | bigint) {
  try {
    return BigInt(value).toLocaleString('en-US')
  } catch {
    return '-'
  }
}

function LogTokenUsage({ item }: { item: LogItem }) {
  const { t } = useTranslation()
  const cacheRead = tokenBigInt(item.cache_read_tokens)
  const cacheWrite = cacheWriteTokens(item)
  return (
    <div className='grid gap-0.5 font-mono text-xs tabular-nums'>
      <span>
        {formatTokenCount(item.prompt_tokens)} /{' '}
        {formatTokenCount(item.completion_tokens)}
      </span>
      {cacheRead > 0n || cacheWrite > 0n ? (
        <span className='text-muted-foreground/60 text-[11px] leading-none'>
          {cacheRead > 0n &&
            `${t('logs.tokens.cacheRead')}↓ ${formatTokenCount(cacheRead)}`}
          {cacheRead > 0n && cacheWrite > 0n ? ' · ' : ''}
          {cacheWrite > 0n && `↑ ${formatTokenCount(cacheWrite)}`}
        </span>
      ) : (
        <span className='text-muted-foreground/50 text-[11px] leading-none'>
          —
        </span>
      )}
    </div>
  )
}

function LogCost({
  quota,
  rate,
  inline = false,
}: {
  quota: MetricString
  rate: RateInfo
  inline?: boolean
}) {
  const { t } = useTranslation()
  const amount = calculateQuotaAmount(quota, rate)
  if (amount.status !== 'available') {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('amount.rateUnavailable')}
      </span>
    )
  }
  const cny = formatDecimal(amount.amountCny, PRECISE_AMOUNT_FRACTION_DIGITS)
  const usd = formatDecimal(amount.amountUsd, PRECISE_AMOUNT_FRACTION_DIGITS)
  if (inline) {
    return (
      <span className='font-mono text-xs font-medium tabular-nums'>
        {t('logs.cost.summary', { cny, usd })}
      </span>
    )
  }
  return (
    <div className='flex flex-col gap-0.5 font-mono text-xs leading-tight font-medium tabular-nums'>
      <span>{t('logs.cost.cny', { amount: cny })}</span>
      <span className='text-muted-foreground/60'>
        {t('logs.cost.usd', { amount: usd })}
      </span>
    </div>
  )
}

type TimingVariant = 'danger' | 'neutral' | 'success' | 'warning'

const timingTextClass: Record<TimingVariant, string> = {
  danger: 'text-destructive',
  neutral: 'text-muted-foreground',
  success: 'text-success',
  warning: 'text-warning',
}

const timingIndicatorClass: Record<TimingVariant, string> = {
  danger: 'bg-destructive/80',
  neutral: 'bg-muted-foreground/60',
  success: 'bg-success/90',
  warning: 'bg-warning/80',
}

function timingNumber(value: string | null) {
  if (value == null) return null
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : null
}

function firstTokenTimingVariant(milliseconds: string | null): TimingVariant {
  const value = timingNumber(milliseconds)
  if (value == null || value <= 0) return 'neutral'
  const seconds = value / 1000
  if (seconds < 5) return 'success'
  if (seconds < 10) return 'warning'
  return 'danger'
}

function durationTimingVariant(item: LogItem): TimingVariant {
  const seconds = timingNumber(item.use_time_seconds)
  const completionTokens = timingNumber(item.completion_tokens)
  if (seconds == null || completionTokens == null) return 'neutral'
  if (completionTokens >= 100 && seconds > 0) {
    const throughput = completionTokens / seconds
    if (throughput >= 30) return 'success'
    if (throughput >= 15) return 'warning'
    return 'danger'
  }
  if (seconds < 10) return 'success'
  if (seconds < 30) return 'warning'
  return 'danger'
}

function LogTiming({
  item,
  indicator = 'bar',
}: {
  item: LogItem
  indicator?: 'bar' | 'dot'
}) {
  const { t } = useTranslation()
  const firstTokenVariant = firstTokenTimingVariant(item.first_response_time_ms)
  const durationVariant = durationTimingVariant(item)
  const labels = (
    <div className='flex min-h-8 min-w-0 flex-col justify-center gap-0.5 text-xs leading-tight'>
      {item.is_stream && (
        <div className='flex items-baseline gap-1.5'>
          {indicator === 'dot' && (
            <span
              aria-hidden
              className={cn(
                'size-1.5 shrink-0 rounded-full',
                timingIndicatorClass[firstTokenVariant]
              )}
            />
          )}
          <span className='text-muted-foreground shrink-0'>
            {t('logs.timing.firstToken')}
          </span>
          <span
            className={cn(
              'font-mono tabular-nums',
              timingTextClass[firstTokenVariant]
            )}
          >
            {formatMilliseconds(item.first_response_time_ms)}
          </span>
        </div>
      )}
      <div className='flex items-baseline gap-1.5'>
        {indicator === 'dot' && (
          <span
            aria-hidden
            className={cn(
              'size-1.5 shrink-0 rounded-full',
              timingIndicatorClass[durationVariant]
            )}
          />
        )}
        <span className='text-muted-foreground shrink-0'>
          {t('logs.timing.duration')}
        </span>
        <span
          className={cn(
            'font-mono tabular-nums',
            timingTextClass[durationVariant]
          )}
        >
          {formatDurationSeconds(item.use_time_seconds)}
        </span>
      </div>
    </div>
  )

  if (indicator === 'dot') return labels

  return (
    <div className='flex min-w-28 items-stretch gap-2'>
      <span
        aria-hidden
        className={cn(
          'flex w-1 shrink-0 flex-col overflow-hidden rounded-full',
          !item.is_stream && timingIndicatorClass[durationVariant]
        )}
      >
        {item.is_stream && (
          <>
            <span
              className={cn('flex-1', timingIndicatorClass[firstTokenVariant])}
            />
            <span
              className={cn('flex-1', timingIndicatorClass[durationVariant])}
            />
          </>
        )}
      </span>
      {labels}
    </div>
  )
}

function StatBadge({
  accent,
  label,
  value,
}: {
  accent: string
  label: string
  value: ReactNode
}) {
  return (
    <span className='border-border/60 bg-muted/25 inline-flex h-7 items-center gap-2 rounded-md border px-2.5 text-xs shadow-xs'>
      <span className={cn('h-3.5 w-0.5 rounded-full', accent)} />
      <span className='text-muted-foreground'>{label}</span>
      <span className='text-foreground/85 font-mono font-semibold tabular-nums'>
        {value}
      </span>
    </span>
  )
}

function formatTokensPerSecond(
  completionTokens: string,
  durationSeconds: string,
  t: LogTranslate
) {
  try {
    const tokens = BigInt(completionTokens)
    const seconds = BigInt(durationSeconds)
    if (tokens <= 0n || seconds <= 0n) return '-'
    return t('logs.timing.tps', {
      value: String((tokens + seconds / 2n) / seconds),
    })
  } catch {
    return '-'
  }
}

function LogFilters({
  global,
  isSearching,
  onChange,
  onSearch,
  search,
  sites,
}: {
  global: boolean
  isSearching: boolean
  onChange: (changes: Partial<LogSearch>) => void
  onSearch: () => void
  search: LogSearch
  sites: SiteListItem[]
}) {
  const { t } = useTranslation()
  const reset = buildLogSearch({ pageSize: search.pageSize })
  const hasActiveFilters = hasFilterChanges(search, reset, [
    'channelId',
    'end',
    'group',
    'modelName',
    'requestId',
    'siteIds',
    'start',
    'tokenName',
    'type',
    'upstreamRequestId',
    'username',
  ])
  const quickRange = getLogQuickRange(search)
  const advancedTextFilter = (
    key: 'group' | 'requestId' | 'tokenName' | 'upstreamRequestId',
    label: string
  ) => (
    <label className='grid gap-1.5'>
      <span className='text-muted-foreground text-xs'>{label}</span>
      <Input
        aria-label={label}
        className='h-8'
        onChange={(event) => onChange({ [key]: event.target.value, page: 1 })}
        value={search[key]}
      />
    </label>
  )
  const advancedCount = [
    search.tokenName !== '',
    search.group !== '',
    search.requestId !== '',
    search.upstreamRequestId !== '',
    quickRange === 'custom',
  ].filter(Boolean).length
  return (
    <form
      aria-label={t('logs.filters.title')}
      className='flex min-w-0 flex-wrap items-center gap-2'
      onSubmit={(event) => {
        event.preventDefault()
        onSearch()
      }}
    >
      <label className='relative min-w-48 flex-1 sm:max-w-72'>
        <span className='sr-only'>{t('logs.fields.username')}</span>
        <HugeiconsIcon
          className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2'
          icon={Search01Icon}
          size={15}
          strokeWidth={2}
        />
        <Input
          aria-label={t('logs.fields.username')}
          className='h-10 pl-8 sm:h-8'
          onChange={(event) =>
            onChange({ page: 1, username: event.target.value })
          }
          placeholder={t('logs.fields.username')}
          value={search.username}
        />
      </label>
      <Input
        aria-label={t('logs.fields.model')}
        className='h-10 min-w-36 flex-1 sm:h-8 sm:max-w-52'
        onChange={(event) =>
          onChange({ modelName: event.target.value, page: 1 })
        }
        placeholder={t('logs.fields.model')}
        value={search.modelName}
      />
      <Input
        aria-label={t('logs.fields.channelId')}
        className='h-10 w-36 sm:h-8'
        inputMode='numeric'
        onChange={(event) => {
          const value = event.target.value
          onChange({
            channelId: isNonNegativeIdString(value)
              ? parseNonNegativeIdString(value)
              : undefined,
            page: 1,
          })
        }}
        placeholder={t('logs.fields.channelId')}
        value={search.channelId ?? ''}
      />
      {global && (
        <FacetedFilter
          clearLabel={t('logs.filters.allSites')}
          onChange={(value) =>
            onChange({
              page: 1,
              siteIds: isIdString(value) ? [parseIdString(value)] : [],
            })
          }
          options={sites.map((site) => ({ label: site.name, value: site.id }))}
          title={t('logs.filters.site')}
          value={search.siteIds.length === 1 ? search.siteIds[0] : ''}
        />
      )}
      <FacetedFilter
        clearLabel={t('logs.filters.allTypes')}
        onChange={(value) =>
          onChange({
            page: 1,
            type: logTypes.includes(Number(value) as LogType)
              ? (Number(value) as LogType)
              : undefined,
          })
        }
        options={logTypes.map((type) => ({
          label: logTypeLabel(type, t),
          value: String(type),
        }))}
        title={t('logs.filters.type')}
        value={search.type == null ? '' : String(search.type)}
      />
      <SelectControl
        aria-label={t('logs.filters.quickRange')}
        className='h-10 w-32 sm:h-8 sm:w-28'
        onChange={(event) => {
          const value = event.target.value
          if (!logQuickRanges.includes(value as LogQuickRange)) return
          onChange({
            ...buildLogQuickRange(value as LogQuickRange),
            page: 1,
          })
        }}
        size='sm'
        value={quickRange}
      >
        <option value='today'>{t('logs.range.today')}</option>
        <option value='24h'>{t('logs.range.hours24')}</option>
        <option value='7d'>{t('logs.range.days7')}</option>
        <option value='14d'>{t('logs.range.days14')}</option>
        <option disabled value='custom'>
          {t('logs.range.custom')}
        </option>
      </SelectControl>
      <Popover>
        <PopoverTrigger
          render={
            <Button
              className='h-10 border-dashed sm:h-8'
              size='sm'
              type='button'
              variant='outline'
            />
          }
        >
          <HugeiconsIcon icon={Settings02Icon} size={15} strokeWidth={2} />
          {t('common.moreFilters')}
          {advancedCount > 0 && (
            <Badge className='px-1.5 font-mono' variant='secondary'>
              {advancedCount}
            </Badge>
          )}
        </PopoverTrigger>
        <PopoverContent
          align='start'
          className='w-[min(640px,calc(100vw-2rem))] p-3'
        >
          <div className='grid gap-3 sm:grid-cols-2'>
            {advancedTextFilter('tokenName', t('logs.fields.token'))}
            {advancedTextFilter('group', t('logs.fields.group'))}
            {advancedTextFilter('requestId', t('logs.fields.requestId'))}
            {advancedTextFilter(
              'upstreamRequestId',
              t('logs.fields.upstreamRequestId')
            )}
            <label className='grid gap-1.5'>
              <span className='text-muted-foreground text-xs'>
                {t('logs.filters.start')}
              </span>
              <Input
                aria-label={t('logs.filters.start')}
                className='h-8'
                onChange={(event) => {
                  const start = parseDateTime(event.target.value)
                  if (start != null) onChange({ page: 1, start })
                }}
                type='datetime-local'
                value={dateTimeValue(search.start)}
              />
            </label>
            <label className='grid gap-1.5'>
              <span className='text-muted-foreground text-xs'>
                {t('logs.filters.end')}
              </span>
              <Input
                aria-label={t('logs.filters.end')}
                className='h-8'
                onChange={(event) => {
                  const end = parseDateTime(event.target.value)
                  if (end != null) onChange({ end, page: 1 })
                }}
                type='datetime-local'
                value={dateTimeValue(search.end)}
              />
            </label>
          </div>
        </PopoverContent>
      </Popover>
      <Button disabled={isSearching} size='sm' type='submit'>
        <HugeiconsIcon icon={Search01Icon} size={15} strokeWidth={2} />
        {t('common.search')}
      </Button>
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
    </form>
  )
}

function LogDetailDialog({
  item,
  onClose,
}: {
  item: LogItem
  onClose: () => void
}) {
  const { t } = useTranslation()
  const consumptionOnlyLabels = new Set([
    t('logs.fields.model'),
    t('logs.fields.token'),
    t('logs.fields.channelId'),
    t('logs.fields.quota'),
    t('logs.fields.cost'),
    t('logs.fields.promptTokens'),
    t('logs.fields.completionTokens'),
    t('logs.fields.cacheReadTokens'),
    t('logs.fields.cacheCreationTokens'),
    t('logs.fields.cacheCreationTokens5m'),
    t('logs.fields.cacheCreationTokens1h'),
    t('logs.fields.duration'),
    t('logs.fields.firstResponseTime'),
    t('logs.fields.stream'),
    t('logs.fields.streamStatus'),
    t('logs.fields.streamEndReason'),
    t('logs.fields.streamErrorCount'),
  ])
  const values = [
    [t('logs.fields.site'), `${item.site_name} · ${item.site_id}`],
    [
      t('logs.fields.createdAt'),
      fromUnixSeconds(item.created_at).format('YYYY-MM-DD HH:mm:ss'),
    ],
    [
      t('logs.fields.remoteUser'),
      `${item.username || '-'} · ${item.remote_user_id}`,
    ],
    [t('logs.fields.model'), item.model_name || '-'],
    [t('logs.fields.token'), `${item.token_name || '-'} · ${item.token_id}`],
    [t('logs.fields.channelId'), item.channel_id],
    [t('logs.fields.group'), item.group || '-'],
    [t('logs.fields.requestId'), item.request_id || '-'],
    [t('logs.fields.upstreamRequestId'), item.upstream_request_id || '-'],
    [t('logs.fields.quota'), item.quota],
    [
      t('logs.fields.cost'),
      <LogCost key='cost' quota={item.quota} rate={item.rate} />,
    ],
    [t('logs.fields.promptTokens'), item.prompt_tokens],
    [t('logs.fields.completionTokens'), item.completion_tokens],
    [t('logs.fields.cacheReadTokens'), item.cache_read_tokens],
    [t('logs.fields.cacheCreationTokens'), item.cache_creation_tokens],
    [t('logs.fields.cacheCreationTokens5m'), item.cache_creation_tokens_5m],
    [t('logs.fields.cacheCreationTokens1h'), item.cache_creation_tokens_1h],
    [t('logs.fields.duration'), formatDurationSeconds(item.use_time_seconds)],
    [
      t('logs.fields.firstResponseTime'),
      formatMilliseconds(item.first_response_time_ms),
    ],
    [
      t('logs.fields.stream'),
      item.is_stream ? t('common.yes') : t('common.no'),
    ],
    [t('logs.fields.streamStatus'), item.stream_status || '-'],
    [t('logs.fields.streamEndReason'), item.stream_end_reason || '-'],
    [t('logs.fields.streamErrorCount'), item.stream_error_count],
    [t('logs.fields.ip'), item.ip || t('logs.notRecorded')],
  ] as const
  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open>
      <DialogContent className='max-h-[calc(100dvh-2rem)] max-w-3xl grid-rows-[auto_minmax(0,1fr)_auto]'>
        <DialogHeader>
          <DialogTitle>{t('logs.detail.title')}</DialogTitle>
          <DialogDescription>{t('logs.detail.description')}</DialogDescription>
        </DialogHeader>
        <div className='grid min-h-0 gap-4 overflow-y-auto pr-1'>
          <div className='flex items-center gap-2'>
            <LogTypeBadge type={item.type} />
            <code className='text-muted-foreground text-xs'>{item.id}</code>
          </div>
          <dl className='grid gap-3 text-sm sm:grid-cols-2'>
            {values.map(([label, value]) =>
              !isConsumptionLogType(item.type) &&
              consumptionOnlyLabels.has(label) ? null : (
                <div className='min-w-0' key={label}>
                  <dt className='text-muted-foreground text-xs'>{label}</dt>
                  <dd className='mt-1 break-all'>{value}</dd>
                </div>
              )
            )}
          </dl>
          <section className='grid gap-2'>
            <h3 className='text-sm font-medium'>{t('logs.fields.content')}</h3>
            <p className='text-muted-foreground text-xs'>
              {t('logs.contentRedacted')}
            </p>
            <pre className='border-border bg-muted/40 max-h-64 overflow-auto rounded-md border p-3 text-xs break-words whitespace-pre-wrap'>
              {formatDisplayValue(item.content)}
            </pre>
          </section>
        </div>
        <DialogFooter>
          <Button onClick={onClose} variant='outline'>
            {t('common.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function LogsPage({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<LogSearch>) => void
  search: LogSearch
  siteId?: string
}) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<LogItem>()
  const [initialJob, setInitialJob] = useState<StatisticsExportJobItem>()
  const validSiteId = siteId == null || isIdString(siteId)
  const params = useMemo(() => logParams(search), [search])
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
  const logsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? listSiteLogs(parseIdString(siteId), params)
        : listLogs(params),
    queryKey:
      siteId && isIdString(siteId)
        ? logKeys.site(siteId, params)
        : logKeys.global(params),
  })
  const statsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      siteId && isIdString(siteId)
        ? getSiteLogStats(parseIdString(siteId), params)
        : getLogStats(params),
    queryKey:
      siteId && isIdString(siteId)
        ? [...logKeys.site(siteId, params), 'stats']
        : [...logKeys.global(params), 'stats'],
  })
  const exportMutation = useMutation({
    mutationFn: ({ format }: { format: StatisticsExportFormat }) =>
      createStatisticsExport(
        buildLogExportRequest(
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
  const data = logsQuery.data
  const stats = statsQuery.data
  const usageAmount = useMemo(
    () =>
      calculateCrossSiteQuotaAmount(
        (stats?.site_breakdown ?? []).map((site) => ({
          quota: site.quota,
          rate: site.rate,
          siteId: site.site_id,
        }))
      ),
    [stats?.site_breakdown]
  )
  let usageValue = t('amount.rateUnavailable')
  if (usageAmount.status === 'available') {
    usageValue = t('logs.cost.summary', {
      cny: formatDecimal(usageAmount.amountCny, PRECISE_AMOUNT_FRACTION_DIGITS),
      usd: formatDecimal(usageAmount.amountUsd, PRECISE_AMOUNT_FRACTION_DIGITS),
    })
  } else if (stats?.quota === '0') {
    usageValue = t('logs.cost.summary', {
      cny: '0.000000',
      usd: '0.000000',
    })
  }
  const columns = useMemo<ColumnDef<LogItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <time className='whitespace-nowrap'>
            {fromUnixSeconds(row.original.created_at).format(
              'YYYY-MM-DD HH:mm:ss'
            )}
          </time>
        ),
        header: t('logs.fields.createdAt'),
        id: 'createdAt',
      },
      {
        cell: ({ row }) => <LogTypeBadge type={row.original.type} />,
        header: t('logs.fields.type'),
        id: 'type',
      },
      {
        cell: ({ row }) => (
          <div className='min-w-36'>
            <span className='font-medium'>{row.original.site_name}</span>
            <code className='text-muted-foreground block text-xs'>
              {row.original.site_id}
            </code>
          </div>
        ),
        header: t('logs.fields.site'),
        id: 'site',
      },
      {
        cell: ({ row }) => (
          <div className='min-w-36'>
            <span>{formatDisplayValue(row.original.username)}</span>
            <code className='text-muted-foreground block text-xs'>
              {row.original.remote_user_id}
            </code>
          </div>
        ),
        header: t('logs.fields.user'),
        id: 'user',
      },
      {
        cell: ({ row }) =>
          isConsumptionLogType(row.original.type) ? (
            <div className='min-w-40'>
              <span>{formatDisplayValue(row.original.model_name)}</span>
              <span className='text-muted-foreground block text-xs'>
                {row.original.token_name || t('logs.tokenUnnamed')} ·{' '}
                {row.original.channel_id}
              </span>
            </div>
          ) : null,
        header: t('logs.fields.modelTokenChannel'),
        id: 'model',
      },
      {
        cell: ({ row }) =>
          isConsumptionLogType(row.original.type) ? (
            <div className='grid min-w-20 gap-1 text-xs'>
              <Badge variant={row.original.is_stream ? 'success' : 'neutral'}>
                {row.original.is_stream
                  ? t('logs.stream.streaming')
                  : t('logs.stream.nonStreaming')}
              </Badge>
              <span className='text-muted-foreground font-mono tabular-nums'>
                {formatTokensPerSecond(
                  row.original.completion_tokens,
                  row.original.use_time_seconds,
                  t
                )}
              </span>
            </div>
          ) : null,
        header: t('logs.fields.mode'),
        id: 'mode',
      },
      {
        cell: ({ row }) =>
          isConsumptionLogType(row.original.type) ? (
            <LogTokenUsage item={row.original} />
          ) : null,
        header: t('logs.fields.tokens'),
        id: 'tokens',
      },
      {
        cell: ({ row }) =>
          isConsumptionLogType(row.original.type) ? (
            <LogCost quota={row.original.quota} rate={row.original.rate} />
          ) : null,
        header: t('logs.fields.cost'),
        id: 'cost',
      },
      {
        cell: ({ row }) =>
          isConsumptionLogType(row.original.type) ? (
            <LogTiming item={row.original} />
          ) : null,
        header: t('logs.fields.timing'),
        id: 'timing',
      },
      {
        cell: ({ row }) => (
          <Button
            aria-label={t('logs.detail.openFor', { id: row.original.id })}
            onClick={() => setSelected(row.original)}
            size='sm'
            variant='outline'
          >
            <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
            {t('common.view')}
          </Button>
        ),
        header: t('common.actions'),
        id: 'actions',
      },
    ],
    [t]
  )
  const actions = (
    <>
      {(['xlsx', 'csv'] as const).map((format) => (
        <Button
          disabled={exportMutation.isPending || !validSiteId}
          key={format}
          onClick={() => exportMutation.mutate({ format })}
          variant='outline'
        >
          <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
          {t('logs.export', { format: format.toUpperCase() })}
        </Button>
      ))}
    </>
  )
  const overviewItems = [
    {
      icon: Chart01Icon,
      label: t('logs.completeness'),
      value: <DataStatusBadge status={data?.data_status ?? 'pending'} />,
    },
    {
      icon: Clock01Icon,
      label: t('logs.asOf'),
      value: data?.as_of
        ? fromUnixSeconds(data.as_of).format('YYYY-MM-DD HH:mm:ss')
        : '-',
    },
    {
      icon: Calendar03Icon,
      label: t('logs.overview.range'),
      value: `${fromUnixSeconds(search.start).format('MM-DD HH:mm')} — ${fromUnixSeconds(search.end).format('MM-DD HH:mm')}`,
    },
  ] as const
  return (
    <SectionPageLayout
      fixedContent
      mobileScrollableContent
      actions={actions}
      description={
        siteId
          ? t('logs.siteDescription', { id: siteId })
          : t('logs.description')
      }
      title={siteId ? t('logs.siteTitle') : t('logs.title')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        {siteId && (
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('logs.backToSite')}
          </DetailBackLink>
        )}
        <div className='flex flex-wrap items-center gap-2'>
          <StatBadge
            accent='bg-violet-500/70'
            label={t('logs.overview.total')}
            value={data?.total ?? 0}
          />
          <StatBadge
            accent='bg-sky-500/70'
            label={t('logs.stats.usage')}
            value={usageValue}
          />
          <StatBadge
            accent='bg-rose-500/65'
            label={t('logs.stats.rpm')}
            value={stats?.rpm ?? '0'}
          />
          <StatBadge
            accent='bg-slate-400/70'
            label={t('logs.stats.tpm')}
            value={stats?.tpm ?? '0'}
          />
        </div>
        <div className='grid gap-3 sm:grid-cols-3'>
          {overviewItems.map(({ icon, label, value }) => (
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
                <dd className='mt-0.5 truncate text-sm font-medium tracking-tight'>
                  {value}
                </dd>
              </dl>
            </div>
          ))}
        </div>
        <section className='border-border bg-muted/30 flex items-start gap-3 rounded-xl border p-4'>
          <span className='bg-background text-muted-foreground ring-foreground/10 flex size-9 shrink-0 items-center justify-center rounded-lg ring-1'>
            <HugeiconsIcon icon={Database01Icon} size={18} strokeWidth={2} />
          </span>
          <div className='min-w-0 flex-1'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <p className='font-medium'>{t('logs.financialNotice.title')}</p>
              <DataStatusBadge status={data?.data_status ?? 'pending'} />
            </div>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('logs.financialNotice.description')}
            </p>
            {data && (
              <p className='text-muted-foreground mt-1 text-xs'>
                {statusDescription(data.data_status, t)}
              </p>
            )}
          </div>
        </section>
        <LogFilters
          global={!siteId}
          isSearching={logsQuery.isFetching}
          onChange={onSearchChange}
          onSearch={() => void logsQuery.refetch()}
          search={search}
          sites={sitesQuery.data?.items ?? []}
        />
        {!siteId && sitesQuery.isError && (
          <QueryStateAlert
            message={t('common.siteOptionsRefreshFailed')}
            onRetry={() => void sitesQuery.refetch()}
          />
        )}
        {logsQuery.isError && data && (
          <QueryStateAlert
            message={t('common.retainedDataRefreshFailed')}
            onRetry={() => void logsQuery.refetch()}
          />
        )}
        {statsQuery.isError && stats && (
          <QueryStateAlert
            message={t('logs.stats.refreshFailed')}
            onRetry={() => void statsQuery.refetch()}
          />
        )}
        <DataTable
          ariaLabel={t('logs.table')}
          columns={columns}
          data={data?.items ?? []}
          emptyDescription={
            data ? statusDescription(data.data_status, t) : undefined
          }
          emptyTitle={t('logs.empty')}
          error={!validSiteId || (logsQuery.isError && !data)}
          fetching={logsQuery.isFetching}
          loading={logsQuery.isPending}
          mobileCardBreakpoint='wide'
          onPageChange={(page) => onSearchChange({ page })}
          onPageSizeChange={(pageSize) => onSearchChange({ page: 1, pageSize })}
          onRetry={validSiteId ? () => void logsQuery.refetch() : undefined}
          page={search.page}
          pageSize={search.pageSize}
          renderMobileCard={(item) => (
            <article className='bg-card text-card-foreground ring-foreground/10 grid gap-3 rounded-xl p-4 ring-1'>
              <div className='flex items-start justify-between gap-3'>
                <div>
                  <time className='text-sm font-medium'>
                    {fromUnixSeconds(item.created_at).format(
                      'YYYY-MM-DD HH:mm:ss'
                    )}
                  </time>
                  <p className='text-muted-foreground text-xs'>
                    {item.site_name} · {item.site_id}
                  </p>
                </div>
                <LogTypeBadge type={item.type} />
              </div>
              <dl className='grid grid-cols-2 gap-3 text-sm'>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('logs.fields.user')}
                  </dt>
                  <dd>{formatDisplayValue(item.username)}</dd>
                </div>
                {isConsumptionLogType(item.type) && (
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('logs.fields.model')}
                    </dt>
                    <dd className='break-all'>{item.model_name || '-'}</dd>
                  </div>
                )}
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('logs.fields.group')}
                  </dt>
                  <dd className='break-all'>{item.group || '-'}</dd>
                </div>
                {isConsumptionLogType(item.type) && (
                  <>
                    <div>
                      <dt className='text-muted-foreground text-xs'>
                        {t('logs.fields.cost')}
                      </dt>
                      <dd className='mt-1'>
                        <LogCost quota={item.quota} rate={item.rate} />
                      </dd>
                    </div>
                    <div>
                      <dt className='text-muted-foreground text-xs'>
                        {t('logs.fields.mode')}
                      </dt>
                      <dd className='mt-1 flex flex-wrap items-center gap-2'>
                        <Badge variant={item.is_stream ? 'success' : 'neutral'}>
                          {item.is_stream
                            ? t('logs.stream.streaming')
                            : t('logs.stream.nonStreaming')}
                        </Badge>
                        <span className='text-muted-foreground font-mono text-xs tabular-nums'>
                          {formatTokensPerSecond(
                            item.completion_tokens,
                            item.use_time_seconds,
                            t
                          )}
                        </span>
                      </dd>
                    </div>
                    <div>
                      <dt className='text-muted-foreground text-xs'>
                        {t('logs.fields.tokens')}
                      </dt>
                      <dd>
                        <LogTokenUsage item={item} />
                      </dd>
                    </div>
                    <div className='col-span-2'>
                      <dt className='text-muted-foreground text-xs'>
                        {t('logs.fields.timing')}
                      </dt>
                      <dd className='mt-1'>
                        <LogTiming indicator='dot' item={item} />
                      </dd>
                    </div>
                  </>
                )}
              </dl>
              <Button onClick={() => setSelected(item)} variant='outline'>
                <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
                {t('common.view')}
              </Button>
            </article>
          )}
          total={data?.total ?? 0}
        />
      </div>
      {selected && (
        <LogDetailDialog
          item={selected}
          onClose={() => setSelected(undefined)}
        />
      )}
      <ExportTaskSheet
        exportId={search.exportId}
        initialJob={initialJob}
        onOpenChange={(open) =>
          !open && onSearchChange({ exportId: undefined })
        }
        onRecreate={(job) => exportMutation.mutate({ format: job.format })}
        recreating={exportMutation.isPending}
      />
    </SectionPageLayout>
  )
}
