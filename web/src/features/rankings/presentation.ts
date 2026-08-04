import Decimal from 'decimal.js'

export function ratioToPercent(value: string | null): string | null {
  return value == null ? null : `${new Decimal(value).times(100).toFixed()}%`
}
