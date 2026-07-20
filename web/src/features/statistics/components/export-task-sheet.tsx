import { Download01Icon, Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
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
import { isIdString } from '@/lib/api-types'
import { formatBeijingTimestamp } from '@/lib/dayjs'
import { translateMessageRef } from '@/lib/message-ref'

import { downloadStatisticsExport, getStatisticsExport } from '../api'
import { statisticsKeys } from '../query-keys'
import type { StatisticsExportJobItem } from '../types'
import {
  exportFormatText,
  exportScopeText,
  ExportStatusBadge,
  ExportTimestamp,
} from './export-ui'

export function ExportTaskSheet({
  exportId,
  initialJob,
  onOpenChange,
  onRecreate,
  recreating,
}: {
  exportId: string | undefined
  initialJob?: StatisticsExportJobItem
  onOpenChange: (open: boolean) => void
  onRecreate: (job: StatisticsExportJobItem) => void
  recreating: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [downloading, setDownloading] = useState(false)
  const [downloadError, setDownloadError] = useState<string>()
  const valid = isIdString(exportId)
  const initialJobForExport =
    initialJob && initialJob.id === exportId ? initialJob : undefined
  const exportQueryKey = statisticsKeys.export(exportId ?? '')
  const cachedJob =
    queryClient.getQueryData<StatisticsExportJobItem>(exportQueryKey)
  const deferInitialRefetch =
    initialJobForExport?.status === 'pending' ||
    initialJobForExport?.status === 'running' ||
    cachedJob?.status === 'pending' ||
    cachedJob?.status === 'running'
  useEffect(() => setDownloadError(undefined), [exportId])
  const jobQuery = useQuery({
    enabled: valid,
    initialData: initialJobForExport,
    queryFn: () => {
      if (!isIdString(exportId)) throw new Error()
      return getStatisticsExport(exportId)
    },
    queryKey: exportQueryKey,
    refetchOnMount: deferInitialRefetch ? false : true,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'pending' || status === 'running' ? 2_000 : false
    },
    staleTime: 2_000,
  })
  const job = jobQuery.data
  const deduplicated = Boolean(
    initialJob && initialJob.id === exportId && initialJob.deduplicated
  )
  const visibleDownloadError =
    downloadError &&
    downloadError !== job?.error?.code &&
    !(job?.status === 'expired' && downloadError === 'EXPORT_EXPIRED')
      ? downloadError
      : undefined
  const download = async () => {
    if (!job || job.status !== 'success') return
    setDownloading(true)
    setDownloadError(undefined)
    try {
      const result = await downloadStatisticsExport(job)
      const url = URL.createObjectURL(result.blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = result.fileName
      anchor.hidden = true
      document.body.append(anchor)
      anchor.click()
      anchor.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 0)
    } catch (error) {
      const key = getApiErrorTranslationKey(error)
      setDownloadError(key)
      toast.error(t(dynamicI18nKey('api', key)))
      void queryClient.invalidateQueries({
        queryKey: statisticsKeys.exportLists(),
      })
      void jobQuery.refetch()
    } finally {
      setDownloading(false)
    }
  }
  return (
    <Sheet onOpenChange={onOpenChange} open={valid}>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t('statistics.export.task.title')}</SheetTitle>
          <SheetDescription>
            {t('statistics.export.task.description')}
          </SheetDescription>
        </SheetHeader>
        {deduplicated && (
          <p
            className='border-primary/30 bg-primary/5 rounded-md border p-3 text-sm'
            role='status'
          >
            {t('statistics.export.task.deduplicated')}
          </p>
        )}
        {jobQuery.isPending && (
          <div
            className='flex min-h-48 items-center justify-center'
            role='status'
          >
            <Spinner />
          </div>
        )}
        {jobQuery.isError && (
          <section className='border-destructive/30 bg-destructive/5 grid gap-3 rounded-md border p-4'>
            <p className='text-destructive text-sm' role='alert'>
              {t('statistics.export.task.loadError')}
            </p>
            <Button onClick={() => void jobQuery.refetch()} variant='outline'>
              <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
              {t('common.retry')}
            </Button>
          </section>
        )}
        {job && (
          <div className='grid gap-5'>
            <section className='border-border grid gap-4 border-y py-4'>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <div>
                  <p className='text-muted-foreground text-xs'>
                    {t('statistics.export.task.id')}
                  </p>
                  <p className='font-medium'>{job.id}</p>
                </div>
                <ExportStatusBadge status={job.status} />
              </div>
              {(job.status === 'pending' || job.status === 'running') && (
                <div className='grid gap-1'>
                  <div className='text-muted-foreground flex justify-between text-xs'>
                    <span>{t('statistics.export.task.progress')}</span>
                    <span>{job.progress}%</span>
                  </div>
                  <progress
                    aria-label={t('statistics.export.task.progress')}
                    className='accent-primary h-2 w-full'
                    max={100}
                    value={job.progress}
                  />
                </div>
              )}
              <dl className='grid gap-3 text-sm sm:grid-cols-2'>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('statistics.export.format')}
                  </dt>
                  <dd>{exportFormatText(t, job.format)}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('statistics.export.scope')}
                  </dt>
                  <dd>{exportScopeText(t, job.statistics_type)}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('statistics.export.task.createdAt')}
                  </dt>
                  <dd>
                    <ExportTimestamp value={job.created_at} />
                  </dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('statistics.export.task.startedAt')}
                  </dt>
                  <dd>
                    <ExportTimestamp value={job.started_at} />
                  </dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('statistics.export.task.finishedAt')}
                  </dt>
                  <dd>
                    <ExportTimestamp value={job.finished_at} />
                  </dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('statistics.export.task.rows')}
                  </dt>
                  <dd>{job.row_count}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('statistics.export.task.size')}
                  </dt>
                  <dd>{job.file_size}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('statistics.export.task.snapshotAt')}
                  </dt>
                  <dd>
                    <ExportTimestamp value={job.data_snapshot_at} />
                  </dd>
                </div>
                <div>
                  <dt className='text-muted-foreground text-xs'>
                    {t('statistics.export.task.expiresAt')}
                  </dt>
                  <dd>
                    <ExportTimestamp value={job.expires_at} />
                  </dd>
                </div>
                <div className='sm:col-span-2'>
                  <dt className='text-muted-foreground text-xs'>
                    {t('statistics.export.task.fileName')}
                  </dt>
                  <dd className='break-words'>
                    {job.file_name || t('exports.value.notGenerated')}
                  </dd>
                </div>
                <div className='sm:col-span-2'>
                  <dt className='text-muted-foreground text-xs'>
                    {t('statistics.export.range')}
                  </dt>
                  <dd>
                    {formatBeijingTimestamp(
                      job.filters.start_timestamp,
                      job.filters.granularity
                    )}{' '}
                    -{' '}
                    {formatBeijingTimestamp(
                      job.filters.end_timestamp,
                      job.filters.granularity
                    )}
                  </dd>
                </div>
              </dl>
            </section>
            {job.status === 'expired' && (
              <p
                className='border-destructive/30 bg-destructive/5 rounded-md border p-3 text-sm'
                role='alert'
              >
                {t('statistics.export.task.expired')}
              </p>
            )}
            {visibleDownloadError && (
              <p
                className='border-destructive/30 bg-destructive/5 rounded-md border p-3 text-sm'
                role='alert'
              >
                {t(dynamicI18nKey('api', visibleDownloadError))}
              </p>
            )}
            {job.status === 'success' && (
              <Button disabled={downloading} onClick={() => void download()}>
                {downloading ? (
                  <Spinner />
                ) : (
                  <HugeiconsIcon icon={Download01Icon} strokeWidth={2} />
                )}
                {t('statistics.export.task.download')}
              </Button>
            )}
            {job.status === 'failed' && job.error && (
              <section
                className='border-destructive/30 bg-destructive/5 rounded-md border p-3 text-sm'
                role='alert'
              >
                <p>{translateMessageRef(job.error)}</p>
                {job.error.technical_detail && (
                  <details className='mt-2'>
                    <summary className='min-h-10 cursor-pointer py-2 font-medium'>
                      {t('statistics.export.task.technical')}
                    </summary>
                    <p className='break-words whitespace-pre-wrap'>
                      {job.error.technical_detail}
                    </p>
                  </details>
                )}
              </section>
            )}
            {(job.status === 'failed' || job.status === 'expired') && (
              <Button
                disabled={recreating}
                onClick={() => onRecreate(job)}
                variant='outline'
              >
                {recreating && <Spinner />}
                {t('statistics.export.task.recreate')}
              </Button>
            )}
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}
