declare const idStringBrand: unique symbol
declare const nonNegativeIdStringBrand: unique symbol
declare const metricStringBrand: unique symbol
declare const decimalStringBrand: unique symbol

export type IdString = string & { readonly [idStringBrand]: true }
export type NonNegativeIdString = string & {
  readonly [nonNegativeIdStringBrand]: true
}
export type MetricString = string & { readonly [metricStringBrand]: true }
export type DecimalString = string & { readonly [decimalStringBrand]: true }
export type Timestamp = number

export type FieldErrorValue = string | readonly string[]
export type FieldErrors = Readonly<Record<string, FieldErrorValue>>

export interface ApiResponse<TData> {
  success: boolean
  message: string
  code: string
  data: TData
  request_id: string
  field_errors?: FieldErrors | null
  params?: Readonly<Record<string, unknown>> | null
}

export interface PageData<TItem> {
  page: number
  page_size: number
  total: number
  items: TItem[]
}

export interface ListQuery {
  p?: number
  page_size?: number
  keyword?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export type DataStatus =
  | 'complete'
  | 'partial'
  | 'pending'
  | 'missing'
  | 'unavailable'
  | 'paused'
  | 'backfilling'
  | 'disabled'

export interface RateInfo {
  quota_per_unit: string | null
  usd_exchange_rate: string | null
  source: 'site' | 'fallback' | 'unavailable'
  updated_at: Timestamp | null
}

const positiveIntegerPattern = /^[1-9]\d*$/
const nonNegativeIntegerPattern = /^(?:0|[1-9]\d*)$/
const signedIntegerPattern = /^-?(?:0|[1-9]\d*)$/
const decimalPattern = /^-?(?:0|[1-9]\d*)(?:\.\d{1,10})?$/

export function isIdString(value: unknown): value is IdString {
  return typeof value === 'string' && positiveIntegerPattern.test(value)
}

export function isNonNegativeIdString(
  value: unknown
): value is NonNegativeIdString {
  return typeof value === 'string' && nonNegativeIntegerPattern.test(value)
}

export function isMetricString(value: unknown): value is MetricString {
  return typeof value === 'string' && signedIntegerPattern.test(value)
}

export function isDecimalString(value: unknown): value is DecimalString {
  return typeof value === 'string' && decimalPattern.test(value)
}

export function parseIdString(value: string): IdString {
  if (!isIdString(value)) throw new TypeError('Expected a positive decimal ID')
  return value
}

export function parseNonNegativeIdString(value: string): NonNegativeIdString {
  if (!isNonNegativeIdString(value)) {
    throw new TypeError('Expected a non-negative decimal ID')
  }
  return value
}

export function parseMetricString(value: string): MetricString {
  if (!isMetricString(value)) {
    throw new TypeError('Expected a decimal integer metric')
  }
  return value
}

export function parseDecimalString(value: string): DecimalString {
  if (!isDecimalString(value)) {
    throw new TypeError('Expected a canonical decimal string')
  }
  return value
}
