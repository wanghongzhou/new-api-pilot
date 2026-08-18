import { keepPreviousData, useQuery } from '@tanstack/react-query'
import type { ColumnDef } from '@tanstack/react-table'
import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { DataTable } from '@/components/ui/data-table'
import { SelectControl as Select } from '@/components/ui/select-control'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { isIdString, parseIdString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'

import { listSiteFastTaskHistory } from '../api'
import {
  collectionTaskCatalog,
  fastCollectionTaskTypes,
  isFastCollectionTaskType,
} from '../constants'
import { siteKeys } from '../query-keys'
import type {
  FastCollectionTaskType,
  FastTaskHistoryItem,
  FastTaskHistoryListParams,
} from '../types'

const fastTaskHistoryPageSize = 10

function StatusBadge({ status }: { status: FastTaskHistoryItem['status'] }) {
  const { t } = useTranslation()
  let variant: 'success' | 'destructive' | 'primary' = 'primary'
  if (status === 'success') variant = 'success'
  else if (status === 'failed') variant = 'destructive'
  return (
    <Badge variant={variant}>
      {t(dynamicI18nKey('site', `collection.status.${status}`))}
    </Badge>
  )
}

function FastTaskHistoryCard({ item }: { item: FastTaskHistoryItem }) {
  const { t } = useTranslation()
  return (
    <article className='bg-card text-card-foreground ring-foreground/10 rounded-xl p-4 ring-1'>
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0'>
          <h3 className='font-medium'>
            {t(dynamicI18nKey('site', `collection.task.${item.task_type}`))}
          </h3>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t(
              dynamicI18nKey(
                'site',
                collectionTaskCatalog[item.task_type].purposeKey
              )
            )}
          </p>
        </div>
        <StatusBadge status={item.status} />
      </div>
      <div className='mt-3 grid grid-cols-2 gap-3 text-sm'>
        <dl className='col-span-2'>
          <dt className='text-muted-foreground'>{t('collection.taskId')}</dt>
          <dd className='break-all'>{item.request_id}</dd>
        </dl>
        <dl>
          <dt className='text-muted-foreground'>{t('collection.startedAt')}</dt>
          <dd>
            {fromUnixSeconds(item.started_at).format('YYYY-MM-DD HH:mm:ss')}
          </dd>
        </dl>
        <dl>
          <dt className='text-muted-foreground'>
            {t('collection.finishedAt')}
          </dt>
          <dd>
            {fromUnixSeconds(item.finished_at).format('YYYY-MM-DD HH:mm:ss')}
          </dd>
        </dl>
        <dl>
          <dt className='text-muted-foreground'>{t('collection.duration')}</dt>
          <dd>
            {t('collection.durationSeconds', {
              value: (item.duration_ms / 1000).toFixed(1),
            })}
          </dd>
        </dl>
      </div>
      {item.error && (
        <p className='text-destructive mt-3 text-sm break-words'>
          {item.error}
        </p>
      )}
    </article>
  )
}

interface FastTaskHistorySearch {
  fastPage: number
  fastStatus?: FastTaskHistoryItem['status']
  fastTaskType: FastCollectionTaskType
}

export function FastTaskHistoryPanel({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<FastTaskHistorySearch>) => void
  search: FastTaskHistorySearch
  siteId: string
}) {
  const { t } = useTranslation()
  const validSiteId = isIdString(siteId)
  const params = useMemo<FastTaskHistoryListParams>(
    () => ({
      site_id: parseIdString(validSiteId ? siteId : '1'),
      task_type: search.fastTaskType,
      status: search.fastStatus ?? '',
      offset: (search.fastPage - 1) * fastTaskHistoryPageSize,
      limit: fastTaskHistoryPageSize,
    }),
    [
      search.fastPage,
      search.fastStatus,
      search.fastTaskType,
      siteId,
      validSiteId,
    ]
  )
  const query = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () => listSiteFastTaskHistory(params),
    queryKey: siteKeys.fastTaskHistory(siteId, params),
    refetchInterval: 5_000,
    staleTime: 5_000,
  })
  const rawItems = query.data?.items ?? []
  const contractError = rawItems.some(
    (item) => !isFastCollectionTaskType(item.task_type)
  )
  const items = contractError ? [] : rawItems
  const total = query.data?.total ?? 0
  useEffect(() => {
    if (!query.data || total === 0) return
    const lastPage = Math.max(1, Math.ceil(total / fastTaskHistoryPageSize))
    if (search.fastPage > lastPage) onSearchChange({ fastPage: lastPage })
  }, [onSearchChange, query.data, search.fastPage, total])
  const columns = useMemo<ColumnDef<FastTaskHistoryItem, unknown>[]>(
    () => [
      {
        accessorKey: 'request_id',
        header: t('collection.taskId'),
      },
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
          fromUnixSeconds(row.original.started_at).format(
            'YYYY-MM-DD HH:mm:ss'
          ),
        header: t('collection.startedAt'),
        id: 'startedAt',
      },
      {
        cell: ({ row }) =>
          fromUnixSeconds(row.original.finished_at).format(
            'YYYY-MM-DD HH:mm:ss'
          ),
        header: t('collection.finishedAt'),
        id: 'finishedAt',
      },
      {
        cell: ({ row }) =>
          t('collection.durationSeconds', {
            value: (row.original.duration_ms / 1000).toFixed(1),
          }),
        header: t('collection.duration'),
        id: 'duration',
      },
      {
        cell: ({ row }) => <StatusBadge status={row.original.status} />,
        header: t('collection.status'),
        id: 'status',
      },
      {
        cell: ({ row }) => row.original.error || '-',
        header: t('collection.error'),
        id: 'error',
      },
    ],
    [t]
  )
  return (
    <section className='grid gap-4' id='fast-task-history'>
      <div className='flex flex-wrap items-end justify-between gap-3'>
        <div className='min-w-0 flex-1'>
          <h2 className='text-lg font-semibold'>
            {t('collection.fastHistoryTitle')}
          </h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('collection.fastHistoryDescription')}
          </p>
        </div>
        <div className='flex shrink-0 flex-wrap items-center justify-end gap-x-4 gap-y-2'>
          <div className='flex items-center gap-2'>
            <span className='text-muted-foreground text-sm whitespace-nowrap'>
              {t('collection.status')}
            </span>
            <Select
              aria-label={t('collection.filterStatus')}
              className='w-32'
              onChange={(event) => {
                onSearchChange({
                  fastPage: 1,
                  fastStatus:
                    event.target.value === ''
                      ? undefined
                      : (event.target.value as FastTaskHistoryItem['status']),
                })
              }}
              value={search.fastStatus ?? ''}
            >
              <option value=''>{t('common.allStatuses')}</option>
              <option value='running'>{t('collection.status.running')}</option>
              <option value='success'>{t('collection.status.success')}</option>
              <option value='failed'>{t('collection.status.failed')}</option>
            </Select>
          </div>
          <div className='flex items-center gap-2'>
            <span className='text-muted-foreground text-sm whitespace-nowrap'>
              {t('collection.taskType')}
            </span>
            <Select
              aria-label={t('collection.filterTaskType')}
              className='w-48'
              onChange={(event) => {
                onSearchChange({
                  fastPage: 1,
                  fastTaskType: event.target.value as FastCollectionTaskType,
                })
              }}
              value={search.fastTaskType}
            >
              {fastCollectionTaskTypes.map((type) => (
                <option key={type} value={type}>
                  {t(dynamicI18nKey('site', `collection.task.${type}`))}
                </option>
              ))}
            </Select>
          </div>
        </div>
      </div>
      {contractError && (
        <p className='text-destructive text-sm' role='alert'>
          {t('collection.contract.unknownFastTask')}
        </p>
      )}
      <DataTable
        ariaLabel={t('collection.fastHistoryTable')}
        columns={columns}
        data={items}
        emptyDescription={t('collection.fastHistoryEmptyDescription')}
        emptyTitle={t('collection.fastHistoryEmpty')}
        error={!validSiteId || query.isError || contractError}
        fetching={query.isFetching}
        fillAvailableHeight={false}
        loading={query.isPending}
        onPageChange={(fastPage) => onSearchChange({ fastPage })}
        onRetry={() => void query.refetch()}
        page={search.fastPage}
        pageSize={fastTaskHistoryPageSize}
        paginationInFooter={false}
        renderMobileCard={(item) => <FastTaskHistoryCard item={item} />}
        total={total}
      />
    </section>
  )
}
