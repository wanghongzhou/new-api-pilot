import { stripSearchParams } from '@tanstack/react-router'
import Decimal from 'decimal.js'
import { z } from 'zod'

import {
  alertLevels,
  alertRuleCategories,
  alertRuleScopes,
  alertRuleSortFields,
  alertSortFields,
  alertStatuses,
  alertTabs,
  alertTargetTypes,
} from './constants'
import type { AlertRuleFormValues, AlertRuleItem } from './types'

const optionalId = z
  .preprocess(
    (value) => (value === '' || value == null ? undefined : value),
    z
      .string()
      .regex(/^[1-9]\d*$/)
      .optional()
  )
  .catch(undefined)

const optionalTimestamp = z
  .preprocess(
    (value) => (value === '' || value == null ? undefined : value),
    z.coerce.number().int().nonnegative().optional()
  )
  .catch(undefined)

function searchArray<T extends readonly [string, ...string[]]>(values: T) {
  return z
    .preprocess(
      (value) => {
        if (value == null) return []
        return Array.isArray(value) ? value : [value]
      },
      z
        .array(z.enum(values))
        .transform(
          (selected) =>
            values.filter((value) => selected.includes(value)) as T[number][]
        )
    )
    .catch([])
}

function singleSearchArray<T extends readonly [string, ...string[]]>(
  values: T
) {
  return searchArray(values).transform((selected) => selected.slice(0, 1))
}

const optionalBoolean = z
  .preprocess((value) => {
    if (value === 'true') return true
    if (value === 'false') return false
    return value
  }, z.boolean().optional())
  .catch(undefined)

export const alertsSearchSchema = z.object({
  alertId: optionalId,
  end: optionalTimestamp,
  level: searchArray(alertLevels),
  order: z.enum(['asc', 'desc']).optional().catch(undefined),
  page: z.coerce.number().int().min(1).optional().catch(undefined),
  pageSize: z.coerce.number().int().min(1).max(100).optional().catch(undefined),
  ruleCategory: singleSearchArray(alertRuleCategories),
  ruleEnabled: optionalBoolean,
  ruleInherited: optionalBoolean,
  ruleLevel: singleSearchArray(alertLevels),
  ruleOrder: z.enum(['asc', 'desc']).optional().catch(undefined),
  rulePage: z.coerce.number().int().min(1).optional().catch(undefined),
  rulePageSize: z.coerce
    .number()
    .int()
    .min(1)
    .max(100)
    .optional()
    .catch(undefined),
  ruleSiteId: optionalId,
  ruleSort: z.enum(alertRuleSortFields).optional().catch(undefined),
  scope: z.enum(alertRuleScopes).optional().catch(undefined),
  siteId: optionalId,
  sort: z.enum(alertSortFields).optional().catch(undefined),
  start: optionalTimestamp,
  status: searchArray(alertStatuses),
  tab: z.enum(alertTabs).optional().catch(undefined),
  targetType: searchArray(alertTargetTypes),
})

type AlertsSearchParams = z.output<typeof alertsSearchSchema>

export const alertSearchMiddlewares = [
  stripSearchParams<AlertsSearchParams>({
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
    tab: 'events',
    targetType: [],
  }),
]

const canonicalDecimal = /^(?:0|[1-9]\d*)(?:\.[0-9]{1,2})?$/
const canonicalForTimes = /^(?:[1-9]|[1-5][0-9]|60)$/

function decimalValue(value: string): Decimal | null {
  if (!canonicalDecimal.test(value)) return null
  const integer = value.split('.')[0]
  if (integer.length > 20) return null
  try {
    return new Decimal(value)
  } catch {
    return null
  }
}

export function createAlertRuleFormSchema(
  rule: AlertRuleItem,
  pairedRule?: AlertRuleItem
) {
  return z
    .object({
      enabled: z.boolean(),
      forTimes: z.string(),
      thresholdValue: z.string(),
    })
    .superRefine((values, context) => {
      if (rule.constraints.for_times_editable) {
        if (!canonicalForTimes.test(values.forTimes)) {
          context.addIssue({
            code: 'custom',
            message: 'alerts.validation.forTimes',
            path: ['forTimes'],
          })
        }
      }

      if (!rule.constraints.threshold_editable) return
      const threshold = decimalValue(values.thresholdValue)
      if (!threshold) {
        context.addIssue({
          code: 'custom',
          message: values.thresholdValue
            ? 'alerts.validation.thresholdFormat'
            : 'alerts.validation.thresholdRequired',
          path: ['thresholdValue'],
        })
        return
      }
      const minimum = rule.constraints.threshold_min
      const maximum = rule.constraints.threshold_max
      if (
        (minimum != null && threshold.lt(new Decimal(minimum))) ||
        (maximum != null && threshold.gt(new Decimal(maximum)))
      ) {
        context.addIssue({
          code: 'custom',
          message: 'alerts.validation.thresholdRange',
          path: ['thresholdValue'],
        })
      }
      if (!pairedRule?.threshold_value) return
      const paired = decimalValue(pairedRule.threshold_value)
      if (!paired) return
      const invalidPair =
        (rule.level === 'warning' && threshold.gte(paired)) ||
        (rule.level === 'critical' && threshold.lte(paired))
      if (invalidPair) {
        context.addIssue({
          code: 'custom',
          message: 'alerts.validation.warningLessCritical',
          path: ['thresholdValue'],
        })
      }
    })
}

export type AlertRuleFormOutput = AlertRuleFormValues
