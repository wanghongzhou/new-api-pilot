import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import {
  alertSearchMiddlewares,
  alertsSearchSchema,
  createAlertRuleFormSchema,
} from './schema'
import type { AlertRuleItem } from './types'

function ruleFixture(
  level: AlertRuleItem['level'],
  thresholdValue: string,
  overrides: Partial<AlertRuleItem> = {}
): AlertRuleItem {
  const id = parseIdString(level === 'warning' ? '11' : '12')
  return {
    base_rule_id: id,
    category: 'instance',
    compare_operator: '>=',
    constraints: {
      for_times_editable: true,
      for_times_max: 60,
      for_times_min: 1,
      paired_rule_id: parseIdString(level === 'warning' ? '12' : '11'),
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
    threshold_value: thresholdValue,
    updated_at: 1,
    ...overrides,
  }
}

describe('alert schemas', () => {
  test('strips inactive filters and other defaults from the browser URL', () => {
    const search = alertsSearchSchema.parse({
      level: [],
      order: 'desc',
      page: 1,
      pageSize: 20,
      ruleCategory: [],
      ruleLevel: [],
      ruleOrder: 'asc',
      rulePage: 1,
      rulePageSize: 20,
      scope: 'global',
      status: [],
      tab: 'rules',
      targetType: [],
    })
    const normalized = alertSearchMiddlewares[0]({
      next: (nextSearch) => nextSearch,
      search,
    })

    expect(normalized as Record<string, unknown>).toEqual({ tab: 'rules' })
  })

  test('accepts metric rule sorting and drops removed consecutive-count sorting', () => {
    expect(alertsSearchSchema.parse({ ruleSort: 'metric' }).ruleSort).toBe(
      'metric'
    )
    expect(
      alertsSearchSchema.parse({ ruleSort: 'for_times' }).ruleSort
    ).toBeUndefined()
  })

  test('keeps bigint URL IDs as strings and drops invalid deep-link values', () => {
    const valid = alertsSearchSchema.parse({
      alertId: '9007199254740993',
      page: '2',
      pageSize: '100',
      rulePage: '3',
      rulePageSize: '50',
      ruleSiteId: '9007199254740995',
      siteId: '9007199254740997',
      start: '1783900000',
    })
    expect(valid).toMatchObject({
      alertId: '9007199254740993',
      page: 2,
      pageSize: 100,
      rulePage: 3,
      rulePageSize: 50,
      ruleSiteId: '9007199254740995',
      siteId: '9007199254740997',
      start: 1_783_900_000,
    })

    expect(
      alertsSearchSchema.parse({
        alertId: '0',
        level: 'fatal',
        page: '0',
        pageSize: '101',
        sort: 'created_at',
      })
    ).toEqual({
      alertId: undefined,
      level: [],
      page: undefined,
      pageSize: undefined,
      ruleCategory: [],
      ruleLevel: [],
      rulePage: undefined,
      rulePageSize: undefined,
      ruleSort: undefined,
      sort: undefined,
      status: [],
      targetType: [],
    })
  })

  test('accepts legacy single filters and canonicalizes repeated filters', () => {
    expect(
      alertsSearchSchema.parse({
        level: ['warning', 'critical', 'warning'],
        status: 'firing',
        targetType: ['account', 'site', 'account'],
      })
    ).toMatchObject({
      level: ['critical', 'warning'],
      status: ['firing'],
      targetType: ['site', 'account'],
    })
  })

  test('keeps rule category and level filters single-select', () => {
    expect(
      alertsSearchSchema.parse({
        ruleCategory: ['channel', 'instance'],
        ruleLevel: ['warning', 'critical'],
      })
    ).toMatchObject({
      ruleCategory: ['instance'],
      ruleLevel: ['critical'],
    })
  })

  test.each(['0', '01', '1.0', '61'])(
    'rejects invalid for_times value %s',
    (value) => {
      const rule = ruleFixture('warning', '70')
      expect(
        createAlertRuleFormSchema(
          rule,
          ruleFixture('critical', '80')
        ).safeParse({
          enabled: true,
          forTimes: value,
          thresholdValue: '70',
        }).success
      ).toBeFalse()
    }
  )

  test.each(['', '+70', '070', '7e1', '70.123'])(
    'rejects non-canonical threshold %s',
    (value) => {
      const rule = ruleFixture('warning', '70')
      expect(
        createAlertRuleFormSchema(
          rule,
          ruleFixture('critical', '80')
        ).safeParse({
          enabled: true,
          forTimes: '60',
          thresholdValue: value,
        }).success
      ).toBeFalse()
    }
  )

  test('uses exact decimals and enforces Warning below Critical in both directions', () => {
    const warning = ruleFixture('warning', '0.3')
    const critical = ruleFixture('critical', '0.31')
    const values = { enabled: true, forTimes: '1' }

    expect(
      createAlertRuleFormSchema(warning, critical).safeParse({
        ...values,
        thresholdValue: '0.3',
      }).success
    ).toBeTrue()
    expect(
      createAlertRuleFormSchema(warning, critical).safeParse({
        ...values,
        thresholdValue: '0.31',
      }).success
    ).toBeFalse()
    expect(
      createAlertRuleFormSchema(critical, warning).safeParse({
        ...values,
        thresholdValue: '0.3',
      }).success
    ).toBeFalse()
  })

  test('ignores fixed fields while still validating editable fields', () => {
    const fixed = ruleFixture('info', '0', {
      constraints: {
        ...ruleFixture('info', '0').constraints,
        for_times_editable: false,
        paired_rule_id: null,
        relation: null,
        threshold_editable: false,
      },
    })
    expect(
      createAlertRuleFormSchema(fixed).safeParse({
        enabled: false,
        forTimes: 'invalid',
        thresholdValue: 'invalid',
      }).success
    ).toBeTrue()
  })
})
