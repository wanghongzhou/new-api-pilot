import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { alertRuleSortFields, alertSortFields } from './constants'
import {
  alertListParams,
  alertRuleListParams,
  alertRuleFormValues,
  alertRuleOverrideRequest,
  alertRuleUpdateRequest,
  hasAlertRuleChanges,
  pairedAlertRule,
} from './contract'
import type { AlertRuleItem, AlertSearch } from './types'

function ruleFixture(
  level: AlertRuleItem['level'],
  overrides: Partial<AlertRuleItem> = {}
): AlertRuleItem {
  const id = parseIdString(level === 'warning' ? '21' : '22')
  return {
    base_rule_id: id,
    category: 'instance',
    compare_operator: '>=',
    constraints: {
      for_times_editable: true,
      for_times_max: 60,
      for_times_min: 1,
      paired_rule_id: parseIdString(level === 'warning' ? '22' : '21'),
      relation: 'warning_lt_critical',
      threshold_editable: true,
      threshold_max: '100',
      threshold_min: '0',
      threshold_step: '0.1',
      value_kind: 'percentage',
    },
    editable_fields: ['enabled', 'threshold_value', 'for_times'],
    effective_rule_id: id,
    enabled: true,
    for_times: 3,
    id,
    inherited: false,
    level,
    metric: 'cpu_usage',
    name: 'CPU threshold',
    override_rule_id: null,
    rule_key: 'cpu_high',
    scope_id: '0',
    scope_type: 'global',
    threshold_value: level === 'warning' ? '70' : '80',
    updated_at: 1,
    ...overrides,
  }
}

describe('alert frontend contract', () => {
  test('exposes only business-comparable event sorts', () => {
    expect(alertSortFields).toEqual([
      'rule_key',
      'status',
      'level',
      'site_name',
      'first_fired_at',
      'last_fired_at',
      'resolved_at',
    ])
    expect(alertSortFields).not.toContain('current_value')
    expect(alertSortFields).not.toContain('target_name')
  })

  test('does not expose compare operator or consecutive count as rule sorts', () => {
    expect(alertRuleSortFields).not.toContain('compare_operator')
    expect(alertRuleSortFields).not.toContain('for_times')
  })

  test('maps URL state to the documented list query without coercing IDs', () => {
    const search: AlertSearch = {
      alertId: parseIdString('9007199254740993'),
      end: 1_784_000_000,
      level: ['critical', 'warning'],
      order: 'desc',
      page: 2,
      pageSize: 20,
      ruleCategory: ['instance'],
      ruleEnabled: true,
      ruleInherited: false,
      ruleLevel: ['warning'],
      ruleOrder: 'desc',
      rulePage: 3,
      rulePageSize: 10,
      ruleSiteId: parseIdString('9007199254740995'),
      ruleSort: 'updated_at',
      scope: 'site',
      siteId: parseIdString('9007199254740997'),
      sort: 'last_fired_at',
      start: 1_783_900_000,
      status: ['firing', 'pending'],
      tab: 'events',
      targetType: ['site', 'account'],
    }
    expect(alertListParams(search)).toEqual({
      end_timestamp: 1_784_000_000,
      level: ['critical', 'warning'],
      p: 2,
      page_size: 20,
      site_id: parseIdString('9007199254740997'),
      sort_by: 'last_fired_at',
      sort_order: 'desc',
      start_timestamp: 1_783_900_000,
      status: ['firing', 'pending'],
      target_type: ['site', 'account'],
    })
    expect(alertRuleListParams(search)).toEqual({
      category: ['instance'],
      enabled: true,
      inherited: false,
      level: ['warning'],
      p: 3,
      page_size: 10,
      scope_id: parseIdString('9007199254740995'),
      scope_type: 'site',
      sort_by: 'updated_at',
      sort_order: 'desc',
    })
  })

  test('finds only the matching opposite-level rule', () => {
    const warning = ruleFixture('warning')
    const unrelated = ruleFixture('critical', { rule_key: 'memory_high' })
    const critical = ruleFixture('critical')
    expect(pairedAlertRule([warning, unrelated, critical], warning)).toBe(
      critical
    )
    expect(pairedAlertRule([warning, critical], critical)).toBe(warning)
    expect(
      pairedAlertRule([warning, critical], ruleFixture('info'))
    ).toBeUndefined()
  })

  test('builds a minimal global update using editable fields only', () => {
    const rule = ruleFixture('warning')
    const initial = alertRuleFormValues(rule)
    expect(initial).toEqual({
      enabled: true,
      forTimes: '3',
      thresholdValue: '70',
    })
    const request = alertRuleUpdateRequest(
      { enabled: false, forTimes: '4', thresholdValue: '71.50' },
      initial,
      rule
    )
    expect(request).toEqual({
      enabled: false,
      for_times: 4,
      threshold_value: '71.50',
    })
    expect(hasAlertRuleChanges(request)).toBeTrue()
    expect(
      hasAlertRuleChanges(alertRuleUpdateRequest(initial, initial, rule))
    ).toBeFalse()
  })

  test('normalizes database scale without adding threshold precision', () => {
    const rule = ruleFixture('warning', {
      threshold_value: '70.10',
    })
    const values = alertRuleFormValues(rule)

    expect(values.thresholdValue).toBe('70.1')
    expect(alertRuleUpdateRequest(values, values, rule)).toEqual({})
    expect(
      alertRuleOverrideRequest(values, rule, parseIdString('9007199254740993'))
    ).toEqual({
      base_rule_id: parseIdString('21'),
      enabled: true,
      for_times: 3,
      site_id: parseIdString('9007199254740993'),
      threshold_value: '70.1',
    })
  })

  test('omits fixed fields from updates and site overrides', () => {
    const rule = ruleFixture('warning', {
      constraints: {
        ...ruleFixture('warning').constraints,
        for_times_editable: false,
        threshold_editable: false,
      },
    })
    const initial = alertRuleFormValues(rule)
    const values = {
      enabled: false,
      forTimes: '60',
      thresholdValue: '99',
    }
    expect(alertRuleUpdateRequest(values, initial, rule)).toEqual({
      enabled: false,
    })
    expect(
      alertRuleOverrideRequest(values, rule, parseIdString('9007199254740993'))
    ).toEqual({
      base_rule_id: parseIdString('21'),
      enabled: false,
      site_id: parseIdString('9007199254740993'),
    })
  })
})
