import { useTranslation } from 'react-i18next'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

import {
  amount,
  balanceDeltaYuan,
  toYuan,
  coveragePercent,
  sampleTime,
  type RecordItem,
} from './data'
import { OriginalUnit, RecordStatus } from './record-table'

export function CalculationDialog({
  row,
  onClose,
}: {
  row: RecordItem | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const daily = row?.kind === 'daily'
  return (
    <Dialog
      open={row != null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent className='sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>
            {row?.name} ·{' '}
            {daily ? t('Daily report details') : t('Consumption calculation')}
          </DialogTitle>
          <DialogDescription>
            {daily
              ? t('Daily calculation explanation')
              : t('Interval calculation explanation')}
          </DialogDescription>
        </DialogHeader>
        {row && (
          <>
            <div className='bg-muted/50 rounded-lg p-4'>
              <p className='text-muted-foreground text-sm'>
                {daily
                  ? t('Daily consumption CNY')
                  : t('Adjusted consumption CNY')}
              </p>
              <p className='mt-1 text-2xl font-semibold tabular-nums'>
                {amount(row.consumption_yuan)}
              </p>
              <p className='mt-2 text-sm'>
                <RecordStatus row={row} />
              </p>
              {!daily &&
                row.consumption_yuan != null &&
                balanceDeltaYuan(row) != null &&
                toYuan(row.recharge, row.rate) != null && (
                  <p className='mt-3 border-t pt-3 text-sm tabular-nums'>
                    {t('Adjusted CNY equation', {
                      difference: amount(balanceDeltaYuan(row)),
                      recharge: amount(toYuan(row.recharge, row.rate)),
                      consumption: amount(row.consumption_yuan),
                    })}
                  </p>
                )}
            </div>
            {daily ? (
              <dl className='grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)] gap-3 text-sm'>
                <dt className='text-muted-foreground'>{t('Report day')}</dt>
                <dd>{row.day}</dd>
                <dt className='text-muted-foreground'>
                  {t('Effective day coverage')}
                </dt>
                <dd>{coveragePercent(row)}</dd>
                <dt className='text-muted-foreground'>
                  {t('Included sample count')}
                </dt>
                <dd>{row.sample_count ?? '—'}</dd>
                <dt className='text-muted-foreground'>
                  {t('Anomaly sample count')}
                </dt>
                <dd>{row.anomaly_count ?? '—'}</dd>
                <dt className='text-muted-foreground'>
                  {t('Opening sample balance')}
                </dt>
                <dd className='break-words tabular-nums'>
                  {amount(row.opening_balance)}
                  <span className='text-muted-foreground block text-xs'>
                    {sampleTime(row.opening_at)}
                  </span>
                </dd>
                <dt className='text-muted-foreground'>
                  {t('Closing sample balance')}
                </dt>
                <dd className='break-words tabular-nums'>
                  {amount(row.closing_balance)}
                  <span className='text-muted-foreground block text-xs'>
                    {sampleTime(row.closing_at)}
                  </span>
                </dd>
              </dl>
            ) : (
              <>
                <div className='grid gap-3 sm:grid-cols-2'>
                  <div className='rounded-lg border p-3'>
                    <p className='text-muted-foreground'>
                      {t('Interval opening balance')}
                    </p>
                    <p className='mt-1 font-medium break-words tabular-nums'>
                      {amount(row.previous_balance)}
                    </p>
                    <p className='text-muted-foreground text-xs'>
                      {row.previous_balance == null
                        ? t('No previous sample')
                        : sampleTime(row.started_at)}
                    </p>
                  </div>
                  <div className='rounded-lg border p-3'>
                    <p className='text-muted-foreground'>
                      {t('Interval closing balance')}
                    </p>
                    <p className='mt-1 font-medium break-words tabular-nums'>
                      {amount(row.balance)}
                    </p>
                    <p className='text-muted-foreground text-xs'>
                      {sampleTime(row.sampled_at)}
                    </p>
                  </div>
                </div>
                <dl className='grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)] gap-3 text-sm'>
                  <dt className='text-muted-foreground'>
                    {t('Original calculation unit')}
                  </dt>
                  <dd>
                    <OriginalUnit field={row.field} />
                  </dd>
                  <dt className='text-muted-foreground'>
                    {t('Balance delta')}
                  </dt>
                  <dd className='break-words tabular-nums'>
                    {amount(row.balance_delta)}
                  </dd>
                  <dt className='text-muted-foreground'>
                    {t('Confirmed recharge')}
                  </dt>
                  <dd className='break-words tabular-nums'>
                    {amount(row.recharge)}
                  </dd>
                  <dt className='text-muted-foreground'>
                    {t('Adjusted raw consumption')}
                  </dt>
                  <dd className='break-words tabular-nums'>
                    {amount(row.consumption)}
                  </dd>
                  <dt className='text-muted-foreground'>
                    {t('Frozen CNY rate')}
                  </dt>
                  <dd className='break-words tabular-nums'>
                    {row.rate ?? '—'}
                  </dd>
                </dl>
                <p className='rounded-lg border p-3 text-sm leading-relaxed'>
                  {t('Consumption formula')}
                </p>
              </>
            )}
            <p className='text-muted-foreground text-xs'>
              {t('Report received time', { time: sampleTime(row.received_at) })}
            </p>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
