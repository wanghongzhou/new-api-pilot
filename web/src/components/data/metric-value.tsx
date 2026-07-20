import { useTranslation } from 'react-i18next'

import type { DecimalString, MetricString } from '@/lib/api-types'

export function MetricValue({
  value,
  compact = false,
  nullLabel,
}: {
  compact?: boolean
  nullLabel?: string
  value: DecimalString | MetricString | null
}) {
  const { t } = useTranslation()
  if (value == null)
    return <span>{nullLabel ?? t('data.unavailableValue')}</span>

  let display: string = value
  try {
    const amount = BigInt(value)
    display = compact
      ? Intl.NumberFormat('zh-CN', {
          maximumFractionDigits: 1,
          notation: 'compact',
        }).format(amount)
      : amount.toLocaleString('zh-CN')
  } catch {
    display = value
  }

  return <span title={value}>{display}</span>
}
