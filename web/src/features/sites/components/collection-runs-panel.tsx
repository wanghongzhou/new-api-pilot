import { Refresh01Icon, ViewIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { SelectControl as Select } from '@/components/ui/select-control'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { isIdString, parseIdString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import { translateMessageRef } from '@/lib/message-ref'

import {
  backfillSite,
  getCollectionRun,
  listCollectionRunWindows,
  listSiteCollectionRuns,
} from '../api'
import {
  collectionTaskCatalog,
  collectionTaskCategories,
  collectionRunStatuses,
  collectionRunWindowStatuses,
  collectionTaskTypes,
} from '../constants'
import { siteKeys } from '../query-keys'
import {
  canRetrySiteUsageRun,
  siteRunContractError,
  windowsBelongToSiteRun,
} from '../run-contract'
import type {
  CollectionRunItem,
  CollectionRunListParams,
  CollectionRunWindowItem,
  CollectionRunWindowListParams,
  CollectionTaskType,
} from '../types'
import { FastTaskHistoryPanel } from './fast-task-history-panel'

interface CollectionRunsSearch {
  runId?: string
  runPage: number
  runStatus?: CollectionRunItem['status']
  runTaskType?: CollectionTaskType
  windowPage: number
  windowStatus?: CollectionRunWindowItem['status']
}

interface CollectionRunsPanelProps {
  isAdmin: boolean
  onSearchChange: (changes: Partial<CollectionRunsSearch>) => void
  search: CollectionRunsSearch
  siteId: string
}

function formatRange(run: CollectionRunItem): string {
  if (run.start_timestamp == null || run.end_timestamp == null) return '-'
  return `${fromUnixSeconds(run.start_timestamp).format('YYYY-MM-DD HH:00')} - ${fromUnixSeconds(run.end_timestamp).format('YYYY-MM-DD HH:00')}`
}

function formatDuration(run: CollectionRunItem): string {
  if (run.started_at == null) return '-'
  const end = run.finished_at ?? Math.floor(Date.now() / 1000)
  return `${Math.max(0, end - run.started_at)}s`
}

function RunStatusBadge({ status }: { status: CollectionRunItem['status'] }) {
  const { t } = useTranslation()
  let variant: 'neutral' | 'primary' | 'success' | 'destructive' = 'neutral'
  if (status === 'running') variant = 'primary'
  else if (status === 'success') variant = 'success'
  else if (status === 'failed') variant = 'destructive'
  return (
    <Badge variant={variant}>
      {t(dynamicI18nKey('site', `collection.status.${status}`))}
    </Badge>
  )
}

function WindowStatusBadge({
  status,
}: {
  status: CollectionRunWindowItem['status']
}) {
  const { t } = useTranslation()
  let variant: 'neutral' | 'primary' | 'success' | 'destructive' = 'neutral'
  if (status === 'running') variant = 'primary'
  else if (status === 'success') variant = 'success'
  else if (status === 'failed') variant = 'destructive'
  return (
    <Badge variant={variant}>
      {t(dynamicI18nKey('site', `collection.windowStatus.${status}`))}
    </Badge>
  )
}

function RunCard({
  onOpen,
  run,
}: {
  onOpen: () => void
  run: CollectionRunItem
}) {
  const { t } = useTranslation()
  return (
    <article className='bg-card text-card-foreground ring-foreground/10 rounded-xl p-4 ring-1'>
      <div className='flex items-start justify-between gap-3'>
        <div>
          <h3 className='font-medium'>
            {t(dynamicI18nKey('site', `collection.task.${run.task_type}`))}
          </h3>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t(
              dynamicI18nKey(
                'site',
                collectionTaskCatalog[run.task_type].purposeKey
              )
            )}
          </p>
          <p className='text-muted-foreground text-xs'>{formatRange(run)}</p>
        </div>
        <RunStatusBadge status={run.status} />
      </div>
      <dl className='mt-3 grid grid-cols-2 gap-3 text-sm'>
        <div>
          <dt className='text-muted-foreground'>{t('collection.trigger')}</dt>
          <dd>
            {t(
              dynamicI18nKey('site', `collection.trigger.${run.trigger_type}`)
            )}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('collection.progress')}</dt>
          <dd>{Math.round(run.progress * 100)}%</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {t('collection.fetchedRows')}
          </dt>
          <dd>
            <MetricValue value={run.fetched_rows} />
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {t('collection.writtenRows')}
          </dt>
          <dd>
            <MetricValue value={run.written_rows} />
          </dd>
        </div>
      </dl>
      <Button className='mt-3 w-full' onClick={onOpen} variant='outline'>
        <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
        {t('collection.viewWindows')}
      </Button>
    </article>
  )
}

function WindowCard({ window }: { window: CollectionRunWindowItem }) {
  const { t } = useTranslation()
  return (
    <article className='bg-card text-card-foreground ring-foreground/10 rounded-xl p-4 ring-1'>
      <div className='flex items-start justify-between gap-3'>
        <h3 className='font-medium'>
          {fromUnixSeconds(window.hour_ts).format('YYYY-MM-DD HH:00')}
        </h3>
        <WindowStatusBadge status={window.status} />
      </div>
      <dl className='mt-3 grid grid-cols-2 gap-3 text-sm'>
        <div>
          <dt className='text-muted-foreground'>
            {t('collection.factStatus')}
          </dt>
          <dd>
            <DataStatusBadge status={window.fact_status} />
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('collection.attempts')}</dt>
          <dd>{window.attempt_count}</dd>
        </div>
      </dl>
      {window.error && (
        <p className='text-destructive mt-3 text-sm'>
          {translateMessageRef(window.error)}
        </p>
      )}
    </article>
  )
}

function RunWindowsSheet({
  isAdmin,
  onClose,
  onSearchChange,
  runId,
  search,
  siteId,
}: {
  isAdmin: boolean
  onClose: () => void
  onSearchChange: (changes: Partial<CollectionRunsSearch>) => void
  runId: string
  search: CollectionRunsSearch
  siteId: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [retrying, setRetrying] = useState(false)
  const validRunId = isIdString(runId)
  const validSiteId = isIdString(siteId)
  const runQuery = useQuery({
    enabled: validRunId && validSiteId,
    queryFn: () => getCollectionRun(parseIdString(runId)),
    queryKey: siteKeys.run(runId),
    refetchInterval: (query) => {
      const run = query.state.data
      return run?.status === 'pending' || run?.status === 'running'
        ? 5_000
        : false
    },
    staleTime: 5_000,
  })
  const run = runQuery.data
  const runContractError = run ? siteRunContractError(run, siteId) : null
  const ownedRun = run != null && runContractError == null
  const windowParams = useMemo<CollectionRunWindowListParams>(
    () => ({
      p: search.windowPage,
      page_size: 20,
      status: search.windowStatus,
    }),
    [search.windowPage, search.windowStatus]
  )
  const windowsQuery = useQuery({
    enabled: validRunId && validSiteId && ownedRun,
    placeholderData: keepPreviousData,
    queryFn: () => listCollectionRunWindows(parseIdString(runId), windowParams),
    queryKey: siteKeys.windows(runId, windowParams),
    refetchInterval: (query) => {
      const windows = query.state.data?.items ?? []
      return windows.some(
        (window) => window.status === 'pending' || window.status === 'running'
      )
        ? 5_000
        : false
    },
    staleTime: 5_000,
  })
  const rawWindows = windowsQuery.data?.items ?? []
  const windowsContractError =
    rawWindows.length > 0 && !windowsBelongToSiteRun(rawWindows, runId, siteId)
  const windows = windowsContractError ? [] : rawWindows

  const retryFailed = async () => {
    if (!run || !canRetrySiteUsageRun(run, siteId) || !validSiteId) return
    const startTimestamp = run.start_timestamp
    const endTimestamp = run.end_timestamp
    if (startTimestamp == null || endTimestamp == null) return
    setRetrying(true)
    try {
      await backfillSite(parseIdString(siteId), {
        end_timestamp: endTimestamp,
        only_missing: true,
        start_timestamp: startTimestamp,
      })
      toast.success(t('collection.retryQueued'))
      void queryClient.invalidateQueries({ queryKey: siteKeys.all })
    } catch (error) {
      toast.error(t(dynamicI18nKey('site', getApiErrorTranslationKey(error))))
    } finally {
      setRetrying(false)
    }
  }

  const columns = useMemo<ColumnDef<CollectionRunWindowItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) =>
          fromUnixSeconds(row.original.hour_ts).format('YYYY-MM-DD HH:00'),
        header: t('collection.hour'),
        id: 'hour',
      },
      {
        cell: ({ row }) => <WindowStatusBadge status={row.original.status} />,
        header: t('collection.executionStatus'),
        id: 'status',
      },
      {
        cell: ({ row }) => (
          <DataStatusBadge status={row.original.fact_status} />
        ),
        header: t('collection.factStatus'),
        id: 'fact',
      },
      {
        accessorKey: 'attempt_count',
        header: t('collection.attempts'),
      },
      {
        cell: ({ row }) =>
          row.original.next_retry_at == null
            ? '-'
            : fromUnixSeconds(row.original.next_retry_at).format(
                'YYYY-MM-DD HH:mm:ss'
              ),
        header: t('collection.nextRetry'),
        id: 'nextRetry',
      },
      {
        cell: ({ row }) =>
          row.original.verified_at == null
            ? '-'
            : fromUnixSeconds(row.original.verified_at).format(
                'YYYY-MM-DD HH:mm:ss'
              ),
        header: t('collection.verifiedAt'),
        id: 'verified',
      },
      {
        cell: ({ row }) =>
          row.original.error ? translateMessageRef(row.original.error) : '-',
        header: t('collection.error'),
        id: 'error',
      },
    ],
    [t]
  )

  return (
    <Sheet onOpenChange={(open) => !open && onClose()} open>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t('collection.windowsTitle', { id: runId })}</SheetTitle>
          <SheetDescription>
            {t('collection.windowsDescription')}
          </SheetDescription>
        </SheetHeader>
        {run && !runContractError && (
          <section className='border-border bg-muted/30 rounded-lg border p-4'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <div>
                <strong>
                  {t(
                    dynamicI18nKey('site', `collection.task.${run.task_type}`)
                  )}
                </strong>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {t(
                    dynamicI18nKey(
                      'site',
                      collectionTaskCatalog[run.task_type].purposeKey
                    )
                  )}
                </p>
              </div>
              <RunStatusBadge status={run.status} />
            </div>
            <p className='text-muted-foreground mt-1 text-sm'>
              {formatRange(run)}
            </p>
            {run.error && (
              <div className='mt-3 text-sm'>
                <p className='text-destructive'>
                  {translateMessageRef(run.error)}
                </p>
                <code className='text-muted-foreground mt-1 block text-xs break-all'>
                  {run.error.technical_detail}
                </code>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t('collection.requestId', {
                    requestId: run.last_request_id,
                  })}
                </p>
              </div>
            )}
          </section>
        )}
        {(runContractError || windowsContractError) && (
          <p className='text-destructive text-sm' role='alert'>
            {t(
              dynamicI18nKey(
                'site',
                runContractError ?? 'collection.contract.foreignWindow'
              )
            )}
          </p>
        )}
        <div className='flex flex-wrap items-center gap-2'>
          <Select
            aria-label={t('collection.filterWindowStatus')}
            onChange={(event) =>
              onSearchChange({
                windowPage: 1,
                windowStatus:
                  event.target.value === ''
                    ? undefined
                    : (event.target.value as CollectionRunWindowItem['status']),
              })
            }
            value={search.windowStatus ?? ''}
          >
            <option value=''>{t('common.allStatuses')}</option>
            {collectionRunWindowStatuses.map((status) => (
              <option key={status} value={status}>
                {t(dynamicI18nKey('site', `collection.windowStatus.${status}`))}
              </option>
            ))}
          </Select>
          {isAdmin && run && canRetrySiteUsageRun(run, siteId) && (
            <Button
              disabled={retrying}
              onClick={() => void retryFailed()}
              variant='outline'
            >
              {retrying ? (
                <Spinner />
              ) : (
                <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
              )}
              {t('collection.retryFailedWindows')}
            </Button>
          )}
        </div>
        <DataTable
          ariaLabel={t('collection.windowsTable')}
          columns={columns}
          data={windows}
          emptyDescription={t('collection.windowsEmptyDescription')}
          emptyTitle={t('collection.windowsEmpty')}
          error={
            !validRunId ||
            !validSiteId ||
            runQuery.isError ||
            Boolean(runContractError) ||
            windowsQuery.isError ||
            windowsContractError
          }
          fetching={windowsQuery.isFetching}
          loading={runQuery.isPending || (ownedRun && windowsQuery.isPending)}
          onPageChange={(windowPage) => onSearchChange({ windowPage })}
          onRetry={() => void windowsQuery.refetch()}
          page={search.windowPage}
          paginationInFooter={false}
          pageSize={windowsQuery.data?.page_size ?? 20}
          renderMobileCard={(window) => <WindowCard window={window} />}
          total={windowsQuery.data?.total ?? 0}
        />
      </SheetContent>
    </Sheet>
  )
}

export function CollectionRunsPanel({
  isAdmin,
  onSearchChange,
  search,
  siteId,
}: CollectionRunsPanelProps) {
  const { t } = useTranslation()
  const validSiteId = isIdString(siteId)
  const params = useMemo<CollectionRunListParams>(
    () => ({
      p: search.runPage,
      page_size: 20,
      status: search.runStatus,
      task_type: search.runTaskType || undefined,
    }),
    [search.runPage, search.runStatus, search.runTaskType]
  )
  const runsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () => listSiteCollectionRuns(parseIdString(siteId), params),
    queryKey: siteKeys.runs(siteId, params),
    refetchInterval: (query) => {
      const runs = query.state.data?.items ?? []
      return runs.some(
        (run) => run.status === 'pending' || run.status === 'running'
      )
        ? 5_000
        : false
    },
    staleTime: 5_000,
  })
  const rawRuns = runsQuery.data?.items ?? []
  const listContractError = rawRuns.some(
    (run) => siteRunContractError(run, siteId) != null
  )
  const runs = listContractError ? [] : rawRuns
  const columns = useMemo<ColumnDef<CollectionRunItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) =>
          t(
            dynamicI18nKey('site', `collection.task.${row.original.task_type}`)
          ),
        header: t('collection.taskType'),
        id: 'taskType',
      },
      {
        cell: ({ row }) =>
          t(
            dynamicI18nKey(
              'site',
              collectionTaskCatalog[row.original.task_type].purposeKey
            )
          ),
        header: t('siteTasks.purposeLabel'),
        id: 'purpose',
      },
      {
        cell: ({ row }) =>
          t(
            dynamicI18nKey(
              'site',
              `collection.trigger.${row.original.trigger_type}`
            )
          ),
        header: t('collection.trigger'),
        id: 'trigger',
      },
      {
        cell: ({ row }) => formatRange(row.original),
        header: t('collection.range'),
        id: 'range',
      },
      {
        cell: ({ row }) => <RunStatusBadge status={row.original.status} />,
        header: t('collection.status'),
        id: 'status',
      },
      {
        cell: ({ row }) => <MetricValue value={row.original.fetched_rows} />,
        header: t('collection.fetchedRows'),
        id: 'fetched',
      },
      {
        cell: ({ row }) => <MetricValue value={row.original.written_rows} />,
        header: t('collection.writtenRows'),
        id: 'written',
      },
      {
        accessorKey: 'retry_count',
        header: t('collection.retries'),
      },
      {
        cell: ({ row }) => formatDuration(row.original),
        header: t('collection.duration'),
        id: 'duration',
      },
      {
        cell: ({ row }) =>
          row.original.error ? translateMessageRef(row.original.error) : '-',
        header: t('collection.error'),
        id: 'error',
      },
      {
        cell: ({ row }) => (
          <Button
            aria-label={t('collection.viewWindows')}
            onClick={() =>
              onSearchChange({ runId: row.original.id, windowPage: 1 })
            }
            size='icon'
            title={t('collection.viewWindows')}
            variant='ghost'
          >
            <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
          </Button>
        ),
        header: t('common.actions'),
        id: 'actions',
      },
    ],
    [onSearchChange, t]
  )

  return (
    <section className='grid gap-4' id='collection-runs'>
      <div className='flex flex-wrap items-end justify-between gap-3'>
        <div>
          <h2 className='text-lg font-semibold'>{t('collection.title')}</h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('collection.description')}
          </p>
        </div>
        <div className='flex flex-wrap gap-2'>
          <Select
            aria-label={t('collection.filterStatus')}
            onChange={(event) =>
              onSearchChange({
                runPage: 1,
                runStatus:
                  event.target.value === ''
                    ? undefined
                    : (event.target.value as CollectionRunItem['status']),
              })
            }
            value={search.runStatus ?? ''}
          >
            <option value=''>{t('common.allStatuses')}</option>
            {collectionRunStatuses.map((status) => (
              <option key={status} value={status}>
                {t(dynamicI18nKey('site', `collection.status.${status}`))}
              </option>
            ))}
          </Select>
          <Select
            aria-label={t('collection.filterTaskType')}
            onChange={(event) =>
              onSearchChange({
                runPage: 1,
                runTaskType:
                  (event.target.value as CollectionTaskType) || undefined,
              })
            }
            value={search.runTaskType ?? ''}
          >
            <option value=''>{t('collection.allTaskTypes')}</option>
            {collectionTaskCategories.map((category) => (
              <optgroup
                key={category}
                label={t(
                  dynamicI18nKey('site', `siteTasks.category.${category}`)
                )}
              >
                {collectionTaskTypes
                  .filter(
                    (taskType) =>
                      collectionTaskCatalog[taskType].category === category
                  )
                  .map((taskType) => (
                    <option key={taskType} value={taskType}>
                      {t(dynamicI18nKey('site', `collection.task.${taskType}`))}
                    </option>
                  ))}
              </optgroup>
            ))}
          </Select>
        </div>
      </div>
      {listContractError && (
        <p className='text-destructive text-sm' role='alert'>
          {t('collection.contract.foreignRun')}
        </p>
      )}
      <DataTable
        ariaLabel={t('collection.table')}
        columns={columns}
        data={runs}
        emptyDescription={t('collection.emptyDescription')}
        emptyTitle={t('collection.empty')}
        error={!validSiteId || runsQuery.isError || listContractError}
        fetching={runsQuery.isFetching}
        loading={runsQuery.isPending}
        onPageChange={(runPage) => onSearchChange({ runPage })}
        onRetry={() => void runsQuery.refetch()}
        page={search.runPage}
        pageSize={runsQuery.data?.page_size ?? 20}
        paginationInFooter={false}
        renderMobileCard={(run) => (
          <RunCard
            onOpen={() => onSearchChange({ runId: run.id, windowPage: 1 })}
            run={run}
          />
        )}
        total={runsQuery.data?.total ?? 0}
      />
      {search.runId && (
        <RunWindowsSheet
          isAdmin={isAdmin}
          onClose={() => onSearchChange({ runId: undefined })}
          onSearchChange={onSearchChange}
          runId={search.runId}
          search={search}
          siteId={siteId}
        />
      )}
      <FastTaskHistoryPanel siteId={siteId} />
    </section>
  )
}
