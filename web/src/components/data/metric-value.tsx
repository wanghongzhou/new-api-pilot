import {
  formatMetricDisplayValue,
  formatNumericDisplayValue,
} from '@/lib/display-value'

export function MetricValue({
  value,
  compact = false,
  nullLabel,
}: {
  compact?: boolean
  nullLabel?: string
  value: string | null
}) {
  if (value == null) {
    return <span>{nullLabel ?? formatNumericDisplayValue(value)}</span>
  }

  return <span title={value}>{formatMetricDisplayValue(value, compact)}</span>
}
