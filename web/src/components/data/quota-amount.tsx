import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { calculateQuotaAmount, formatDecimal } from '@/lib/amount'
import type { MetricString, RateInfo } from '@/lib/api-types'

import { MetricValue } from './metric-value'

export function QuotaAmount({
  inline = false,
  showQuota = true,
  quota,
  rate,
  nullLabel,
}: {
  inline?: boolean
  nullLabel?: string
  quota: MetricString | null
  rate: RateInfo
  showQuota?: boolean
}) {
  const { t } = useTranslation()
  const amount = useMemo(() => calculateQuotaAmount(quota, rate), [quota, rate])
  return (
    <div
      className={
        inline
          ? 'flex min-w-0 flex-wrap items-baseline gap-x-1 gap-y-0.5'
          : 'grid gap-0.5'
      }
    >
      {showQuota && (
        <span>
          <MetricValue compact nullLabel={nullLabel} value={quota} />
          <span className='text-muted-foreground ml-1 text-xs'>
            {t('metric.quota')}
          </span>
        </span>
      )}
      {amount.status === 'available' && (
        <span className='text-muted-foreground text-xs'>
          {t('amount.summary', {
            cny: formatDecimal(amount.amountCny),
            usd: formatDecimal(amount.amountUsd),
          })}
        </span>
      )}
      {amount.status !== 'available' &&
        (quota != null || nullLabel == null) && (
          <span className='text-muted-foreground text-xs'>
            {t('amount.rateUnavailable')}
          </span>
        )}
    </div>
  )
}
