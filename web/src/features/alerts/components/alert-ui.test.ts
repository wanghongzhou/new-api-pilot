import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import { builtInAlertRuleKeys } from '../constants'
import { alertEventTargetLabel, alertEventTargetName } from '../event-text'
import { alertRuleDescription, alertRuleName } from './alert-ui'

describe('alert rule UI text', () => {
  test('maps every built-in rule to a dedicated business-description key', () => {
    const keys = builtInAlertRuleKeys.map((ruleKey) =>
      alertRuleDescription((key) => key, ruleKey)
    )

    expect(keys).toHaveLength(builtInAlertRuleKeys.length)
    expect(new Set(keys).size).toBe(builtInAlertRuleKeys.length)
    for (const key of keys) {
      expect(key).toStartWith('alerts.ruleDescription.')
    }
  })

  test('distinguishes warning and critical names for paired rules', () => {
    const t = (key: string, params?: Record<string, unknown>) =>
      key === 'alerts.rule.withLevel'
        ? `${String(params?.name)}:${String(params?.level)}`
        : key
    const warning = alertRuleName(t, 'cpu_high', 'warning')
    const critical = alertRuleName(t, 'cpu_high', 'critical')

    expect(warning).not.toBe(critical)
    expect(warning).toContain('alerts.level.warning')
    expect(critical).toContain('alerts.level.critical')
  })
})

describe('alert event target text', () => {
  const t = (key: string, params?: Record<string, unknown>) =>
    params ? `${key}:${String(Object.values(params)[0])}` : key

  test('renders collection windows as Beijing hours instead of raw keys', () => {
    const event = {
      rule_key: 'collection_missing',
      target_key: '1/1781323200',
      target_name: '1/1781323200',
      target_type: 'collection' as const,
    }
    expect(alertEventTargetLabel(t, event)).toBe(
      'alerts.target.collectionWindow'
    )
    expect(alertEventTargetName(t, event)).toStartWith(
      'alerts.target.collectionWindowValue:2026-06-13'
    )
  })

  test('renders backfills with the stable run id', () => {
    const event = {
      rule_key: 'backfill_failed',
      target_key: '1/9007199254740993',
      target_name: '1/9007199254740993',
      target_type: 'collection' as const,
    }
    expect(alertEventTargetLabel(t, event)).toBe('alerts.target.backfillRun')
    expect(alertEventTargetName(t, event)).toBe(
      'alerts.target.backfillRunValue:9007199254740993'
    )
  })
})

test('keeps event filters flat and summary in shared metric cards', async () => {
  const [filters, page] = await Promise.all([
    readFile(new URL('./alert-filters.tsx', import.meta.url), 'utf8'),
    readFile(new URL('./alerts-page.tsx', import.meta.url), 'utf8'),
  ])

  expect(filters).not.toContain('advanced=')
  expect(filters.match(/<FacetedFilter/g)).toHaveLength(3)
  expect(filters).toContain("t('alerts.filters.allSites')")
  expect(filters).toContain("<div className='grid gap-1.5 text-sm'>")
  expect(filters).toContain('<AlertDateTimeRangePicker')
  const picker = await readFile(
    new URL('./alert-date-time-range-picker.tsx', import.meta.url),
    'utf8'
  )
  expect(picker).toContain("'alerts.filters.thisWeek'")
  expect(picker).toContain("'alerts.filters.thisMonth'")
  expect(page).toContain("className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'")
  expect(page).toContain('items-center justify-between gap-2')
  expect(page).toContain("search.tab === 'events' && summaryQuery.data")
  expect(page).toContain('summaryQuery.data.updated_at')
  expect(page).toContain('rounded-xl p-4 ring-1')
  expect(page).toContain('Activity03Icon')
  expect(page).toContain('AlertCircleIcon')
  expect(page).toContain('CheckmarkCircle02Icon')
  expect(page).toContain('text-2xl leading-none font-semibold')
  expect(page).toContain("id: 'site_name'")
  expect(page).toContain("id: 'level'")
  expect(page).toContain("id: 'last_fired_at'")
  expect(page).toContain("id: 'resolved_at'")
  expect(page.indexOf("header: t('alerts.table.rule')")).toBeLessThan(
    page.indexOf("header: t('alerts.table.level')")
  )
})
