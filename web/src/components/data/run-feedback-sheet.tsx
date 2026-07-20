import { Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { getCollectionRun } from '@/features/sites/api'
import { siteKeys } from '@/features/sites/query-keys'
import type { CollectionRunItem } from '@/features/sites/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { fromUnixSeconds } from '@/lib/dayjs'
import { translateMessageRef } from '@/lib/message-ref'

import { Badge } from '../ui/badge'
import { Button } from '../ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '../ui/sheet'
import { Spinner } from '../ui/spinner'

function statusVariant(status: CollectionRunItem['status']) {
  if (status === 'success') return 'success' as const
  if (status === 'failed') return 'destructive' as const
  return 'warning' as const
}

function rangeLabel(run: CollectionRunItem): string {
  if (run.start_timestamp == null || run.end_timestamp == null) return '-'
  return `${fromUnixSeconds(run.start_timestamp).format('YYYY-MM-DD HH:00')} - ${fromUnixSeconds(run.end_timestamp).format('YYYY-MM-DD HH:00')}`
}

export function RunFeedbackSheet({
  expectedTargetId,
  expectedTargetType,
  onOpenChange,
  open,
  run: initialRun,
}: {
  expectedTargetId: string
  expectedTargetType: 'account' | 'customer'
  onOpenChange: (open: boolean) => void
  open: boolean
  run: CollectionRunItem | null
}) {
  const { t } = useTranslation()
  const runQuery = useQuery({
    enabled: open && initialRun != null,
    initialData: initialRun ?? undefined,
    queryFn: () => {
      if (!initialRun) throw new TypeError('Collection run is required')
      return getCollectionRun(initialRun.id)
    },
    queryKey: siteKeys.run(initialRun?.id ?? ''),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'pending' || status === 'running' ? 5_000 : false
    },
    staleTime: 5_000,
  })
  const run = runQuery.data
  const validTarget =
    run?.target_type === expectedTargetType &&
    run.target_id === expectedTargetId

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t('runFeedback.title')}</SheetTitle>
          <SheetDescription>{t('runFeedback.description')}</SheetDescription>
        </SheetHeader>
        {runQuery.isPending && (
          <div
            className='flex min-h-48 items-center justify-center'
            role='status'
          >
            <Spinner />
          </div>
        )}
        {(runQuery.isError || (run && !validTarget)) && (
          <section className='border-destructive/30 bg-destructive/5 grid gap-3 rounded-md border p-4'>
            <p className='text-destructive text-sm' role='alert'>
              {t(
                dynamicI18nKey(
                  'data',
                  run && !validTarget
                    ? 'runFeedback.targetMismatch'
                    : 'runFeedback.loadError'
                )
              )}
            </p>
            {runQuery.isError && (
              <Button onClick={() => void runQuery.refetch()} variant='outline'>
                <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
                {t('common.retry')}
              </Button>
            )}
          </section>
        )}
        {run && validTarget && (
          <section className='border-border bg-muted/30 grid gap-4 rounded-md border p-4'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('runFeedback.runId')}
                </p>
                <p className='font-medium'>{run.id}</p>
              </div>
              <Badge variant={statusVariant(run.status)}>
                {t(dynamicI18nKey('data', `collection.status.${run.status}`))}
              </Badge>
            </div>
            <dl className='grid gap-3 text-sm sm:grid-cols-2'>
              <div>
                <dt className='text-muted-foreground'>
                  {t('collection.range')}
                </dt>
                <dd>{rangeLabel(run)}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>
                  {t('collection.progress')}
                </dt>
                <dd>{Math.round(run.progress * 100)}%</dd>
              </div>
            </dl>
            <progress
              aria-label={t('collection.progress')}
              className='accent-primary h-2 w-full'
              max={1}
              value={run.progress}
            />
            {run.error && (
              <div>
                <p className='text-destructive text-sm'>
                  {translateMessageRef(run.error)}
                </p>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t('collection.requestId', {
                    requestId: run.last_request_id,
                  })}
                </p>
              </div>
            )}
          </section>
        )}
      </SheetContent>
    </Sheet>
  )
}
