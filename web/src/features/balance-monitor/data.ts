import Decimal from 'decimal.js'
import { z } from 'zod'

import { requestApiData } from '@/lib/api'

export const balanceSearch = z.object({
  kind: z.enum(['current', 'interval', 'daily', 'monthly']).catch('current'),
  start: z.string().catch(''),
  end: z.string().catch(''),
  account_id: z.string().catch(''),
  site_id: z.string().catch(''),
  p: z.number().int().min(1).catch(1),
})
export type BalanceSearch = z.infer<typeof balanceSearch>
export type RecordItem = {
  monthly?: MonthlyBalanceSnapshot
  opening_balance?: string | null
  closing_balance?: string | null
  opening_at?: string
  closing_at?: string
  source_id: string
  record_id: string
  revision: string
  kind: string
  account_id: string
  site_id: string
  name: string
  site_name: string
  day: string
  sampled_at: string
  received_at: string
  started_at?: string
  field?: string
  balance: string | null
  previous_balance?: string | null
  balance_delta: string | null
  recharge?: string | null
  rate?: string | null
  consumption: string | null
  consumption_yuan: string | null
  balance_yuan: string | null
  coverage_seconds?: string
  sample_count?: string
  anomaly_count?: string
  standalone?: boolean
  flags: string[]
}
export type MonthlyBalanceSnapshot = {
  period: string
  mode: 'scheduled' | 'manual' | 'legacy'
  grand_total_residual_yuan: string
  total_excluding_nowcoding_residual_yuan: string
  foxcode_residual_yuan: string
  other_residual_yuan: string
  nowcoding_residual_yuan: string
  redeem_code_residual_yuan: string
  redeem_code_count: string
  accounts: Array<{ name: string; raw: string; residual_yuan: string }>
  failed: string[]
}
export type RecordPage = { items: RecordItem[]; total: string }
export function records(params: Record<string, string | number>) {
  return requestApiData<RecordPage>({
    method: 'get',
    url: '/api/balance-monitor/records',
    params,
  })
}
export function accounts() {
  return requestApiData<RecordPage>({
    method: 'get',
    url: '/api/balance-monitor/accounts',
  })
}
export function amount(value: string | null | undefined) {
  return value == null ? '—' : new Decimal(value).toFixed(4)
}
export function summarize(
  rows: RecordItem[],
  field: 'balance_yuan' | 'consumption_yuan'
) {
  let total = new Decimal(0)
  let standalone = new Decimal(0)
  let unknown = 0
  let regularKnown = 0
  let corporateKnown = 0
  for (const row of rows) {
    if (row[field] == null) unknown++
    else if (row.standalone) {
      standalone = standalone.add(row[field])
      corporateKnown++
    } else {
      total = total.add(row[field])
      regularKnown++
    }
  }
  return {
    total: regularKnown ? total.toFixed(4) : null,
    standalone: corporateKnown ? standalone.toFixed(4) : null,
    unknown,
  }
}
export function shanghaiDay(offset = 0) {
  return new Date(Date.now() + 8 * 3600000 + offset * 86400000)
    .toISOString()
    .slice(0, 10)
}

export function normalizeBalanceSearch(search: BalanceSearch) {
  if (search.kind === 'monthly') {
    const end = shanghaiDay().slice(0, 7)
    const first = new Date(`${end}-01T00:00:00Z`)
    first.setUTCMonth(first.getUTCMonth() - 11)
    return {
      ...search,
      start: search.start || first.toISOString().slice(0, 7),
      end: search.end || end,
      account_id: '',
      site_id: '',
    }
  }
  const fallback = shanghaiDay(search.kind === 'daily' ? -1 : 0)
  return {
    ...search,
    start: search.start || fallback,
    end: search.end || fallback,
  }
}

export function changeBalanceView(
  search: BalanceSearch,
  kind: BalanceSearch['kind']
): BalanceSearch {
  return normalizeBalanceSearch({ ...search, kind, start: '', end: '', p: 1 })
}

export function validBalanceDates(search: BalanceSearch) {
  if (search.kind === 'current') return true
  if (search.kind === 'monthly') {
    const valid = (value: string) => /^\d{4}-(0[1-9]|1[0-2])$/.test(value)
    if (
      !valid(search.start) ||
      !valid(search.end) ||
      search.start > search.end
    ) {
      return false
    }
    const months = (value: string) =>
      Number(value.slice(0, 4)) * 12 + Number(value.slice(5))
    return months(search.end) - months(search.start) <= 119
  }
  const validDay = (value: string) =>
    /^\d{4}-\d{2}-\d{2}$/.test(value) &&
    !Number.isNaN(Date.parse(value)) &&
    new Date(value).toISOString().slice(0, 10) === value
  return (
    validDay(search.start) &&
    validDay(search.end) &&
    search.end >= search.start &&
    Date.parse(search.end) - Date.parse(search.start) <= 365 * 86400000
  )
}

export function balanceDeltaYuan(row: RecordItem) {
  return toYuan(row.balance_delta, row.rate)
}

export function toYuan(
  value: string | null | undefined,
  rate: string | null | undefined
) {
  if (value == null || rate == null) return null
  return new Decimal(value).mul(rate).toFixed(10)
}

export function intervalMinutes(row: RecordItem) {
  if (row.previous_balance == null || !row.started_at) return null
  const seconds = Number(row.sampled_at) - Number(row.started_at)
  return seconds > 0 ? (seconds / 60).toFixed(1) : null
}

export function coveragePercent(row: RecordItem) {
  if (row.coverage_seconds == null) return '—'
  const seconds = new Decimal(row.coverage_seconds)
  return seconds.lt(0) || seconds.gt(86400)
    ? '—'
    : `${seconds.div(86400).mul(100).toFixed(1)}%`
}

export function sampleTime(value: string | undefined) {
  if (!value) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(new Date(Number(value) * 1000))
}
