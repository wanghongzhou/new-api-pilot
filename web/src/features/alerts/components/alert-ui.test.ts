import { describe, expect, test } from 'bun:test'

import { builtInAlertRuleKeys } from '../constants'
import { alertRuleDescription } from './alert-ui'

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
})
