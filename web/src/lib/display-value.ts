import Decimal from 'decimal.js'

export const EMPTY_DISPLAY_VALUE = '-'
export const EMPTY_NUMERIC_DISPLAY_VALUE = EMPTY_DISPLAY_VALUE

export function formatDisplayValue(
  value: boolean | number | string | null | undefined
): string {
  if (value == null) return EMPTY_DISPLAY_VALUE
  if (typeof value === 'string' && value.trim() === '') {
    return EMPTY_DISPLAY_VALUE
  }
  return String(value)
}

export function formatNumericDisplayValue(
  value: bigint | number | string | null | undefined
): string {
  if (value == null || value === '') return EMPTY_NUMERIC_DISPLAY_VALUE
  return String(value)
}

export function formatDecimalDisplayValue(
  value: string | null | undefined,
  maximumFractionDigits?: number
): string {
  if (value == null || value.trim() === '') return EMPTY_NUMERIC_DISPLAY_VALUE
  try {
    const decimal = new Decimal(value)
    if (!decimal.isFinite()) return EMPTY_NUMERIC_DISPLAY_VALUE
    const displayDecimal =
      maximumFractionDigits == null
        ? decimal
        : decimal.toDecimalPlaces(maximumFractionDigits, Decimal.ROUND_HALF_UP)
    const normalized = displayDecimal.toFixed(displayDecimal.decimalPlaces())
    const [integer, fraction] = normalized.split('.')
    const groupedInteger = (integer ?? '').replaceAll(
      /\B(?=(\d{3})+(?!\d))/g,
      ','
    )
    return fraction ? `${groupedInteger}.${fraction}` : groupedInteger
  } catch {
    return EMPTY_NUMERIC_DISPLAY_VALUE
  }
}
