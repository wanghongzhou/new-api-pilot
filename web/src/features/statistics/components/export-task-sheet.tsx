import { Download01Icon, Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { MetricValue } from '@/components/data/metric-value'
import { QueryStateAlert } from '@/components/data/query-state-alert'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { isIdString } from '@/lib/api-types'
import { formatBeijingTimestamp } from '@/lib/dayjs'
import { formatMetricDisplayValue } from '@/lib/display-value'
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
  const downloadingRef = useRef(false)
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
  const readableFileSize = (value: string) => {
    try {
      const bytes = BigInt(value)
      const units = ['B', 'KB', 'MB', 'GB', 'TB']
      let size = Number(bytes)
      let unit = 0
      while (size >= 1024 && unit < units.length - 1) {
        size /= 1024
        unit += 1
      }
      let precision = 2
      if (unit === 0) precision = 0
      else if (size >= 10) precision = 1
      return `${size.toFixed(precision)} ${units[unit]}`
    } catch {
      return formatMetricDisplayValue(value)
    }
  }
  const filterSummary = job
    ? [
        ['site_ids', job.filters.site_ids],
        ['customer_ids', job.filters.customer_ids],
        ['account_ids', job.filters.account_ids],
        ['model_names', job.filters.model_names],
        ['channel_keys', job.filters.channel_keys],
        ['use_groups', job.filters.use_groups],
        ['token_keys', job.filters.token_keys],
        ['node_names', job.filters.node_names],
      ].filter(([, values]) => Array.isArray(values) && values.length > 0)
    : []
  const granularityLabel = job
    ? {
        day: t('statistics.granularity.day'),
        hour: t('statistics.granularity.hour'),
        month: t('statistics.granularity.month'),
        year: t('statistics.granularity.year'),
      }[job.filters.granularity]
    : ''
  const filterLabels: Record<string, string> = {
    account_ids: t('exports.detail.filter.account_ids'),
    channel_keys: t('exports.detail.filter.channel_keys'),
    customer_ids: t('exports.detail.filter.customer_ids'),
    model_names: t('exports.detail.filter.model_names'),
    node_names: t('exports.detail.filter.node_names'),
    site_ids: t('exports.detail.filter.site_ids'),
    token_keys: t('exports.detail.filter.token_keys'),
    use_groups: t('exports.detail.filter.use_groups'),
  }
  const download = async () => {
    if (!job || job.status !== 'success' || downloadingRef.current) return
    downloadingRef.current = true
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
      downloadingRef.current = false
      setDownloading(false)
    }
  }
  return (
    <Sheet onOpenChange={onOpenChange} open={valid}>
      <SheetContent className='w-full sm:max-w-xl' showCloseButton={false}>
        <SheetHeader>
          <SheetTitle>
            <span className='sr-only'>{t('statistics.export.task.title')}</span>
            <span aria-hidden='true'>{t('exports.detail.title')}</span>
          </SheetTitle>
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
        {jobQuery.isError && !job && (
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
        {jobQuery.isRefetchError && job && (
          <div className='px-4'>
            <QueryStateAlert
              message={t('exports.detail.retainedRefreshFailed')}
              onRetry={() => void jobQuery.refetch()}
            />
          </div>
        )}
        {job && (
          <div
            aria-label={t('exports.detail.scrollRegion')}
            className='min-h-0 flex-1 overflow-y-auto px-4 pb-4'
            tabIndex={0}
          >
            <div className='grid gap-4'>
              <section className='border-border bg-muted/20 grid gap-4 rounded-lg border p-4'>
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
                <div className='grid gap-3 text-sm sm:grid-cols-2'>
                  <dl className='rounded-md border p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('statistics.export.format')}
                    </dt>
                    <dd>{exportFormatText(t, job.format)}</dd>
                  </dl>
                  <dl className='rounded-md border p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('statistics.export.scope')}
                    </dt>
                    <dd>{exportScopeText(t, job.statistics_type)}</dd>
                  </dl>
                  <dl className='rounded-md border p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('statistics.export.task.createdAt')}
                    </dt>
                    <dd>
                      <ExportTimestamp value={job.created_at} />
                    </dd>
                  </dl>
                  <dl className='rounded-md border p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('statistics.export.task.startedAt')}
                    </dt>
                    <dd>
                      <ExportTimestamp value={job.started_at} />
                    </dd>
                  </dl>
                  <dl className='rounded-md border p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('statistics.export.task.finishedAt')}
                    </dt>
                    <dd>
                      <ExportTimestamp value={job.finished_at} />
                    </dd>
                  </dl>
                  <dl className='rounded-md border p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('statistics.export.task.rows')}
                    </dt>
                    <dd>
                      <MetricValue value={job.row_count} />
                    </dd>
                  </dl>
                  <dl className='rounded-md border p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('statistics.export.task.size')}
                    </dt>
                    <dd className='font-medium'>
                      {readableFileSize(job.file_size)}
                      <span className='text-muted-foreground mt-0.5 block text-xs'>
                        {t('exports.detail.exactBytes', {
                          count: formatMetricDisplayValue(job.file_size),
                        })}
                      </span>
                    </dd>
                  </dl>
                  <dl className='rounded-md border p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('statistics.export.task.snapshotAt')}
                    </dt>
                    <dd>
                      <ExportTimestamp value={job.data_snapshot_at} />
                    </dd>
                  </dl>
                  <dl className='rounded-md border p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('statistics.export.task.expiresAt')}
                    </dt>
                    <dd>
                      <ExportTimestamp value={job.expires_at} />
                    </dd>
                  </dl>
                  <dl className='rounded-md border p-3 sm:col-span-2'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('statistics.export.task.fileName')}
                    </dt>
                    <dd className='break-words'>
                      {job.file_name || t('exports.value.notGenerated')}
                    </dd>
                  </dl>
                  {job.filters.start_timestamp > 0 &&
                    job.filters.end_timestamp > job.filters.start_timestamp && (
                      <dl className='rounded-md border p-3 sm:col-span-2'>
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
                      </dl>
                    )}
                </div>
              </section>
              <section className='border-border grid gap-3 rounded-lg border p-4'>
                <div>
                  <h3 className='font-medium'>{t('exports.detail.filters')}</h3>
                  <p className='text-muted-foreground text-xs'>
                    {t('exports.detail.filtersDescription')}
                  </p>
                </div>
                <div className='grid gap-3 text-sm sm:grid-cols-2'>
                  <dl className='rounded-md border p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('statistics.export.granularity')}
                    </dt>
                    <dd>{granularityLabel}</dd>
                  </dl>
                  <dl className='rounded-md border p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('exports.detail.sort')}
                    </dt>
                    <dd>
                      {job.filters.sort_by} · {job.filters.sort_order}
                    </dd>
                  </dl>
                  {filterSummary.length === 0 ? (
                    <dl className='rounded-md border p-3 sm:col-span-2'>
                      <dt className='text-muted-foreground text-xs'>
                        {t('exports.detail.scopeFilters')}
                      </dt>
                      <dd>{t('exports.detail.allData')}</dd>
                    </dl>
                  ) : (
                    filterSummary.map(([key, values]) => (
                      <dl className='rounded-md border p-3' key={String(key)}>
                        <dt className='text-muted-foreground text-xs'>
                          {filterLabels[String(key)]}
                        </dt>
                        <dd className='break-words'>
                          {(values as string[]).join(', ')}
                        </dd>
                      </dl>
                    ))
                  )}
                </div>
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
            </div>
          </div>
        )}
        <SheetFooter className='border-border flex-row flex-wrap justify-start border-t'>
          {job?.status === 'success' && (
            <Button disabled={downloading} onClick={() => void download()}>
              {downloading ? (
                <Spinner />
              ) : (
                <HugeiconsIcon icon={Download01Icon} strokeWidth={2} />
              )}
              {t('statistics.export.task.download')}
            </Button>
          )}
          {job && (job.status === 'failed' || job.status === 'expired') && (
            <Button
              disabled={recreating}
              onClick={() => onRecreate(job)}
              variant='outline'
            >
              {recreating && <Spinner />}
              {t('statistics.export.task.recreate')}
            </Button>
          )}
          <Button onClick={() => onOpenChange(false)} variant='ghost'>
            {t('common.close')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
