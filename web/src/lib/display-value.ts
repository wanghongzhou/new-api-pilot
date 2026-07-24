export const EMPTY_DISPLAY_VALUE = '-'
export const EMPTY_NUMERIC_DISPLAY_VALUE = '0'

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
