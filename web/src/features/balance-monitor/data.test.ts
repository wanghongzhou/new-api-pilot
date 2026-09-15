import { describe, expect, test } from 'bun:test'

import {
  amount,
  balanceDeltaYuan,
  changeBalanceView,
  coveragePercent,
  intervalMinutes,
  normalizeBalanceSearch,
  shanghaiDay,
  summarize,
  validBalanceDates,
  type BalanceSearch,
  type RecordItem,
} from './data'

const search: BalanceSearch = {
  kind: 'daily',
  start: '2026-01-01',
  end: '2026-01-01',
  account_id: 'account',
  site_id: 'site',
  p: 3,
}

describe('balance presentation semantics', () => {
  test('monthly view uses month boundaries and clears account filters', () => {
    const monthly = changeBalanceView(search, 'monthly')
    expect(monthly.start).toHaveLength(7)
    expect(monthly.end).toBe(shanghaiDay().slice(0, 7))
    expect(monthly.account_id).toBe('')
    expect(monthly.site_id).toBe('')
    expect(validBalanceDates(monthly)).toBe(true)
    expect(validBalanceDates({ ...monthly, start: '2026-13' })).toBe(false)
    expect(
      validBalanceDates({ ...monthly, start: '2010-01', end: '2026-01' })
    ).toBe(false)
  })
  test('resets dates and pagination when switching views, retaining account scope', () => {
    const next = changeBalanceView(search, 'interval')
    expect(next).toEqual({
      ...search,
      kind: 'interval',
      start: shanghaiDay(),
      end: shanghaiDay(),
      p: 1,
    })
    expect(changeBalanceView(next, 'daily').start).toBe(shanghaiDay(-1))
    expect(normalizeBalanceSearch({ ...next, start: '', end: '' }).start).toBe(
      shanghaiDay()
    )
  })
  test('validates calendar dates, ordering and bounded range', () => {
    expect(validBalanceDates(search)).toBe(true)
    expect(validBalanceDates({ ...search, start: '2026-02-30' })).toBe(false)
    expect(validBalanceDates({ ...search, start: '2026-01-02' })).toBe(false)
    expect(validBalanceDates({ ...search, end: '2028-01-01' })).toBe(false)
    expect(
      validBalanceDates({ ...search, kind: 'current', start: '', end: '' })
    ).toBe(true)
  })
  test('shows actual interval length and no interval for first sample', () => {
    const row = {
      previous_balance: '10',
      started_at: '1000',
      sampled_at: '2800',
    } as RecordItem
    expect(intervalMinutes(row)).toBe('30.0')
    expect(intervalMinutes({ ...row, previous_balance: null })).toBeNull()
    expect(intervalMinutes({ ...row, sampled_at: '1000' })).toBeNull()
  })
  test('uses the frozen rate for balance differences and preserves negative recharge swings', () => {
    const row = {
      balance_delta: '-2000000000',
      rate: '0.0000002125',
    } as RecordItem
    expect(amount(balanceDeltaYuan(row))).toBe('-425.0000')
    expect(balanceDeltaYuan({ ...row, rate: null })).toBeNull()
    expect(balanceDeltaYuan({ ...row, balance_delta: null })).toBeNull()
  })
  test('preserves precision beyond JavaScript integers', () => {
    expect(amount('9007199254740993.125')).toBe('9007199254740993.1250')
  })
  test('separates unknown and corporate values without presenting unknown totals as zero', () => {
    const rows = [
      { consumption_yuan: '425' },
      { consumption_yuan: null },
      { consumption_yuan: '1.25', standalone: true },
    ] as RecordItem[]
    expect(summarize(rows, 'consumption_yuan')).toEqual({
      total: '425.0000',
      standalone: '1.2500',
      unknown: 1,
    })
    expect(summarize([rows[1]], 'consumption_yuan')).toEqual({
      total: null,
      standalone: null,
      unknown: 1,
    })
    expect(amount(null)).toBe('—')
  })
  test('coverage describes the whole natural day', () => {
    expect(coveragePercent({ coverage_seconds: '43200' } as RecordItem)).toBe(
      '50.0%'
    )
    expect(coveragePercent({ coverage_seconds: '86400' } as RecordItem)).toBe(
      '100.0%'
    )
    expect(coveragePercent({} as RecordItem)).toBe('—')
  })
})
