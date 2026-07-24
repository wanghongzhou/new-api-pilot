import {
  ArrowLeft01Icon,
  FileExportIcon,
  ViewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { FilterPanel } from '@/components/data/filter-panel'
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
import { SelectControl as Select } from '@/components/ui/select-control'
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
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'
import { formatDisplayValue } from '@/lib/display-value'
import { hasFilterChanges } from '@/lib/filter-state'

import { listLogs, listSiteLogs } from '../api'
import { buildLogExportRequest } from '../export-request'
import { logKeys } from '../query-keys'
import { buildLogSearch } from '../search'
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

function LogFilters({
  global,
  onChange,
  search,
}: {
  global: boolean
  onChange: (changes: Partial<LogSearch>) => void
  search: LogSearch
}) {
  const { t } = useTranslation()
  const updateText =
    (key: keyof LogSearch) => (event: ChangeEvent<HTMLInputElement>) =>
      onChange({ [key]: event.target.value, page: 1 })
  const reset = buildLogSearch({ pageSize: search.pageSize })
  return (
    <FilterPanel
      description={t('logs.filters.description')}
      hasActiveFilters={hasFilterChanges(search, reset, [
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
      ])}
      onReset={() => onChange(reset)}
      title={t('logs.filters.title')}
    >
      <div className='grid min-w-0 flex-1 gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <label className='grid gap-1 text-sm'>
          <span>{t('logs.filters.start')}</span>
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
          <span>{t('logs.filters.end')}</span>
          <Input
            onChange={(event) => {
              const end = parseDateTime(event.target.value)
              if (end != null) onChange({ end, page: 1 })
            }}
            type='datetime-local'
            value={dateTimeValue(search.end)}
          />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('logs.filters.type')}</span>
          <Select
            className='w-full'
            onChange={(event) =>
              onChange({
                page: 1,
                type:
                  event.target.value === ''
                    ? undefined
                    : (Number(event.target.value) as LogType),
              })
            }
            value={search.type ?? ''}
          >
            <option value=''>{t('logs.filters.allTypes')}</option>
            {logTypes.map((type) => (
              <option key={type} value={type}>
                {logTypeLabel(type, t)}
              </option>
            ))}
          </Select>
        </label>
        {global && (
          <label className='grid gap-1 text-sm'>
            <span>{t('logs.filters.siteIds')}</span>
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
              placeholder={t('logs.filters.siteIdsPlaceholder')}
              value={search.siteIds.join(',')}
            />
          </label>
        )}
        <label className='grid gap-1 text-sm'>
          <span>{t('logs.fields.username')}</span>
          <Input onChange={updateText('username')} value={search.username} />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('logs.fields.model')}</span>
          <Input onChange={updateText('modelName')} value={search.modelName} />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('logs.fields.token')}</span>
          <Input onChange={updateText('tokenName')} value={search.tokenName} />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('logs.fields.channelId')}</span>
          <Input
            inputMode='numeric'
            onChange={(event) => {
              const value = event.target.value
              if (value === '') onChange({ channelId: undefined, page: 1 })
              else if (isNonNegativeIdString(value)) {
                onChange({
                  channelId: parseNonNegativeIdString(value),
                  page: 1,
                })
              }
            }}
            value={search.channelId ?? ''}
          />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('logs.fields.group')}</span>
          <Input onChange={updateText('group')} value={search.group} />
        </label>
        <label className='grid gap-1 text-sm'>
          <span>{t('logs.fields.requestId')}</span>
          <Input onChange={updateText('requestId')} value={search.requestId} />
        </label>
        <label className='grid gap-1 text-sm xl:col-span-2'>
          <span>{t('logs.fields.upstreamRequestId')}</span>
          <Input
            onChange={updateText('upstreamRequestId')}
            value={search.upstreamRequestId}
          />
        </label>
      </div>
    </FilterPanel>
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
    [t('logs.fields.promptTokens'), item.prompt_tokens],
    [t('logs.fields.completionTokens'), item.completion_tokens],
    [t('logs.fields.duration'), item.use_time_seconds],
    [
      t('logs.fields.stream'),
      item.is_stream ? t('common.yes') : t('common.no'),
    ],
    [t('logs.fields.ip'), item.ip || t('logs.notRecorded')],
  ] as const
  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open>
      <DialogContent className='max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{t('logs.detail.title')}</DialogTitle>
          <DialogDescription>{t('logs.detail.description')}</DialogDescription>
        </DialogHeader>
        <div className='flex items-center gap-2'>
          <LogTypeBadge type={item.type} />
          <code className='text-muted-foreground text-xs'>{item.id}</code>
        </div>
        <dl className='grid gap-3 text-sm sm:grid-cols-2'>
          {values.map(([label, value]) => (
            <div className='min-w-0' key={label}>
              <dt className='text-muted-foreground text-xs'>{label}</dt>
              <dd className='mt-1 break-all'>{value}</dd>
            </div>
          ))}
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
        cell: ({ row }) => (
          <div className='min-w-40'>
            <span>{formatDisplayValue(row.original.model_name)}</span>
            <span className='text-muted-foreground block text-xs'>
              {row.original.token_name || t('logs.tokenUnnamed')} ·{' '}
              {row.original.channel_id}
            </span>
          </div>
        ),
        header: t('logs.fields.modelTokenChannel'),
        id: 'model',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-32 gap-1 text-xs'>
            <span>{t('logs.metric.quota', { value: row.original.quota })}</span>
            <span>
              {t('logs.metric.tokens', {
                completion: row.original.completion_tokens,
                prompt: row.original.prompt_tokens,
              })}
            </span>
            <span>
              {t('logs.metric.duration', {
                value: row.original.use_time_seconds,
              })}
            </span>
          </div>
        ),
        header: t('logs.fields.metrics'),
        id: 'metrics',
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
  return (
    <SectionPageLayout
      fixedContent
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
        <section
          className='border-primary/30 bg-primary/5 rounded-lg border p-4 text-sm'
          role='note'
        >
          <p className='font-medium'>{t('logs.financialNotice.title')}</p>
          <p className='text-muted-foreground mt-1'>
            {t('logs.financialNotice.description')}
          </p>
        </section>
        <LogFilters
          global={!siteId}
          onChange={onSearchChange}
          search={search}
        />
        {data && (
          <section
            className='border-border flex flex-wrap items-center justify-between gap-3 rounded-lg border p-4'
            role='status'
          >
            <div className='flex flex-wrap items-center gap-2'>
              <span className='text-sm font-medium'>
                {t('logs.completeness')}
              </span>
              <DataStatusBadge status={data.data_status} />
              <span className='text-muted-foreground text-sm'>
                {statusDescription(data.data_status, t)}
              </span>
            </div>
            <DataFreshness labelKey='logs.asOf' timestamp={data.as_of} />
          </section>
        )}
        <DataTable
          ariaLabel={t('logs.table')}
          columns={columns}
          data={data?.items ?? []}
          emptyDescription={
            data ? statusDescription(data.data_status, t) : undefined
          }
          emptyTitle={t('logs.empty')}
          error={!validSiteId || logsQuery.isError}
          fetching={logsQuery.isFetching}
          loading={logsQuery.isPending}
          onPageChange={(page) => onSearchChange({ page })}
          onPageSizeChange={(pageSize) => onSearchChange({ page: 1, pageSize })}
          onRetry={() => void logsQuery.refetch()}
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
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('logs.fields.model')}
                  </dt>
                  <dd className='break-all'>{item.model_name || '-'}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('logs.fields.group')}
                  </dt>
                  <dd className='break-all'>{item.group || '-'}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('logs.fields.quota')}
                  </dt>
                  <dd>{item.quota}</dd>
                </div>
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
