import { keepPreviousData, useQuery } from '@tanstack/react-query'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { DataTable } from '@/components/ui/data-table'
import { NativeSelect as Select } from '@/components/ui/native-select'
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
import type { FastTaskHistoryItem } from '../types'

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

export function FastTaskHistoryPanel({ siteId }: { siteId: string }) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [taskType, setTaskType] =
    useState<FastTaskHistoryItem['task_type']>('site_probe')
  const [status, setStatus] = useState<FastTaskHistoryItem['status'] | ''>('')
  const validSiteId = isIdString(siteId)
  const params = useMemo(
    () => ({
      site_id: parseIdString(siteId),
      task_type: taskType,
      status,
      offset: (page - 1) * 50,
      limit: 50,
    }),
    [page, siteId, status, taskType]
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
  const columns = useMemo<ColumnDef<FastTaskHistoryItem, unknown>[]>(
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
        cell: ({ row }) => `${(row.original.duration_ms / 1000).toFixed(1)}s`,
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
    <section className='grid gap-4 border-t pt-5' id='fast-task-history'>
      <div className='flex flex-wrap items-end justify-between gap-3'>
        <div>
          <h2 className='text-lg font-semibold'>
            {t('collection.fastHistoryTitle')}
          </h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('collection.fastHistoryDescription')}
          </p>
        </div>
        <div className='flex flex-wrap gap-2'>
          <Select
            aria-label={t('collection.filterStatus')}
            onChange={(event) => {
              setPage(1)
              setStatus(
                event.target.value as FastTaskHistoryItem['status'] | ''
              )
            }}
            value={status}
          >
            <option value=''>{t('common.allStatuses')}</option>
            <option value='running'>{t('collection.status.running')}</option>
            <option value='success'>{t('collection.status.success')}</option>
            <option value='failed'>{t('collection.status.failed')}</option>
          </Select>
          <Select
            aria-label={t('collection.filterTaskType')}
            onChange={(event) => {
              setPage(1)
              setTaskType(
                event.target.value as FastTaskHistoryItem['task_type']
              )
            }}
            value={taskType}
          >
            {fastCollectionTaskTypes.map((type) => (
              <option key={type} value={type}>
                {t(dynamicI18nKey('site', `collection.task.${type}`))}
              </option>
            ))}
          </Select>
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
        loading={query.isPending}
        onPageChange={setPage}
        onRetry={() => void query.refetch()}
        page={page}
        pageSize={50}
        total={query.data?.total ?? 0}
      />
    </section>
  )
}
