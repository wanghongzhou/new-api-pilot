import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import type { BackfillSummary } from '@/features/sites/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { translateMessageRef } from '@/lib/message-ref'

export function BackfillProgress({ backfill }: { backfill: BackfillSummary }) {
  const { t } = useTranslation()
  if (backfill.status === 'none') return null
  const percentage = Math.round(backfill.progress * 1000) / 10
  const statusKey = {
    failed: 'backfill.status.failed',
    pending: 'backfill.status.pending',
    running: 'backfill.status.running',
  }[backfill.status]
  return (
    <section className='border-border bg-muted/35 rounded-lg border p-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='flex items-center gap-2'>
          <h2 className='font-medium'>{t('backfill.title')}</h2>
          <Badge
            variant={backfill.status === 'failed' ? 'destructive' : 'neutral'}
          >
            {t(dynamicI18nKey('backfill', statusKey))}
          </Badge>
        </div>
        <span className='text-sm font-medium'>{percentage}%</span>
      </div>
      <progress
        aria-label={t('backfill.progress')}
        className='accent-primary mt-2 h-2 w-full'
        max={1}
        value={backfill.progress}
      />
      <p className='text-muted-foreground mt-2 text-xs'>
        {t('backfill.windows', {
          complete: backfill.completed_windows,
          failed: backfill.failed_windows,
          total: backfill.total_windows,
        })}
      </p>
      {backfill.latest_error && (
        <p className='text-destructive mt-2 text-sm'>
          {translateMessageRef(backfill.latest_error)}
        </p>
      )}
    </section>
  )
}
