import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataStatusBadge } from '@/components/data/data-status'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import type { Completeness } from '@/features/sites/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import type { DataStatus, IdString } from '@/lib/api-types'
import { formatBeijingTimestamp } from '@/lib/dayjs'

import type {
  StatisticsExportFormat,
  StatisticsScope,
  StatisticsSearch,
} from '../types'

export function ExportDialog({
  completeness,
  entityId,
  onConfirm,
  onOpenChange,
  pending,
  scope,
  search,
  summaryStatus,
}: {
  completeness: Completeness
  entityId?: IdString
  onConfirm: (format: StatisticsExportFormat) => void
  onOpenChange: (open: boolean) => void
  pending: boolean
  scope: StatisticsScope
  search: StatisticsSearch
  summaryStatus: DataStatus
}) {
  const { t } = useTranslation()
  const [format, setFormat] = useState<StatisticsExportFormat>('xlsx')
  const incomplete =
    summaryStatus !== 'complete' || completeness.data_status !== 'complete'
  const [incompleteConfirmed, setIncompleteConfirmed] = useState(!incomplete)
  let scopeLabel: string
  if (entityId && scope === 'customer') {
    scopeLabel = t('statistics.export.scopeCustomer', { id: entityId })
  } else if (entityId && scope === 'account') {
    scopeLabel = t('statistics.export.scopeAccount', { id: entityId })
  } else {
    scopeLabel = t(dynamicI18nKey('statistics', `statistics.scope.${scope}`))
  }
  return (
    <Dialog onOpenChange={onOpenChange} open>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('statistics.export.dialog.title')}</DialogTitle>
          <DialogDescription>
            {t('statistics.export.dialog.description')}
          </DialogDescription>
        </DialogHeader>
        <dl className='border-border grid gap-3 border-y py-4 text-sm sm:grid-cols-2'>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('statistics.export.scope')}
            </dt>
            <dd className='mt-1 font-medium'>{scopeLabel}</dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('statistics.export.granularity')}
            </dt>
            <dd className='mt-1 font-medium'>
              {t(
                dynamicI18nKey(
                  'statistics',
                  `statistics.granularity.${search.granularity}`
                )
              )}
            </dd>
          </div>
          <div className='sm:col-span-2'>
            <dt className='text-muted-foreground text-xs'>
              {t('statistics.export.range')}
            </dt>
            <dd className='mt-1 font-medium'>
              {formatBeijingTimestamp(search.start, search.granularity)} -{' '}
              {formatBeijingTimestamp(search.end, search.granularity)}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('statistics.export.sort')}
            </dt>
            <dd className='mt-1 font-medium'>
              {t(
                dynamicI18nKey('statistics', `statistics.sort.${search.sort}`)
              )}{' '}
              /{' '}
              {t(
                dynamicI18nKey('statistics', `statistics.order.${search.order}`)
              )}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('statistics.export.completeness')}
            </dt>
            <dd className='mt-1 flex flex-wrap items-center gap-2'>
              <DataStatusBadge status={completeness.data_status} />
              <span>
                {completeness.complete_site_count}/
                {completeness.expected_site_count}
              </span>
            </dd>
          </div>
        </dl>
        <fieldset className='grid gap-2'>
          <legend className='text-sm font-medium'>
            {t('statistics.export.format')}
          </legend>
          <div className='border-border flex w-fit rounded-md border p-0.5'>
            {(['xlsx', 'csv'] as const).map((value) => (
              <Button
                aria-pressed={format === value}
                key={value}
                onClick={() => setFormat(value)}
                type='button'
                variant={format === value ? 'secondary' : 'ghost'}
              >
                {t(
                  dynamicI18nKey(
                    'statistics',
                    `statistics.export.format.${value}`
                  )
                )}
              </Button>
            ))}
          </div>
        </fieldset>
        <div className='border-border bg-muted/30 grid gap-1 rounded-md border p-3 text-sm'>
          <p>{t('statistics.export.dialog.notPaginated')}</p>
          <p>{t('statistics.export.dialog.ratesFrozen')}</p>
          <p>{t('statistics.export.dialog.background')}</p>
        </div>
        {incomplete && (
          <section className='border-warning/40 bg-warning/10 grid gap-3 rounded-md border p-3'>
            <div>
              <p className='font-medium'>
                {t('statistics.export.incomplete.title')}
              </p>
              <p className='text-muted-foreground mt-1 text-sm'>
                {t('statistics.export.incomplete.description')}
              </p>
            </div>
            <label className='flex min-h-10 items-start gap-3 text-sm'>
              <input
                checked={incompleteConfirmed}
                className='accent-primary mt-0.5 size-4'
                onChange={(event) =>
                  setIncompleteConfirmed(event.target.checked)
                }
                type='checkbox'
              />
              <span>{t('statistics.export.incomplete.confirm')}</span>
            </label>
          </section>
        )}
        <DialogFooter>
          <Button
            disabled={pending}
            onClick={() => onOpenChange(false)}
            type='button'
            variant='outline'
          >
            {t('common.cancel')}
          </Button>
          <Button
            disabled={pending || !incompleteConfirmed}
            onClick={() => onConfirm(format)}
            type='button'
          >
            {pending && <Spinner />}
            {t('statistics.export.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
