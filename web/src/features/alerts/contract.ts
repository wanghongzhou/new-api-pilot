import type { IdString } from '@/lib/api-types'

import type {
  AlertListParams,
  AlertRuleFormValues,
  AlertRuleItem,
  AlertRuleOverrideRequest,
  AlertRuleUpdateRequest,
  AlertSearch,
} from './types'

export function alertListParams(search: AlertSearch): AlertListParams {
  return {
    end_timestamp: search.end,
    level: search.level.length > 0 ? search.level : undefined,
    p: search.page,
    page_size: search.pageSize,
    site_id: search.siteId,
    sort_by: search.sort,
    sort_order: search.order,
    start_timestamp: search.start,
    status: search.status.length > 0 ? search.status : undefined,
    target_type: search.targetType.length > 0 ? search.targetType : undefined,
  }
}

export function alertRuleFormValues(rule: AlertRuleItem): AlertRuleFormValues {
  return {
    enabled: rule.enabled,
    forTimes: String(rule.for_times),
    thresholdValue: rule.threshold_value ?? '',
  }
}

export function pairedAlertRule(
  rules: readonly AlertRuleItem[],
  rule: AlertRuleItem
): AlertRuleItem | undefined {
  if (rule.level !== 'warning' && rule.level !== 'critical') return undefined
  const pairedLevel = rule.level === 'warning' ? 'critical' : 'warning'
  return rules.find(
    (candidate) =>
      candidate.rule_key === rule.rule_key && candidate.level === pairedLevel
  )
}

export function alertRuleUpdateRequest(
  values: AlertRuleFormValues,
  initial: AlertRuleFormValues,
  rule: AlertRuleItem
): AlertRuleUpdateRequest {
  const request: AlertRuleUpdateRequest = {}
  if (values.enabled !== initial.enabled) request.enabled = values.enabled
  if (
    rule.constraints.threshold_editable &&
    values.thresholdValue !== initial.thresholdValue
  ) {
    request.threshold_value = values.thresholdValue
  }
  if (
    rule.constraints.for_times_editable &&
    values.forTimes !== initial.forTimes
  ) {
    request.for_times = Number(values.forTimes)
  }
  return request
}

export function alertRuleOverrideRequest(
  values: AlertRuleFormValues,
  rule: AlertRuleItem,
  siteId: IdString
): AlertRuleOverrideRequest {
  const request: AlertRuleOverrideRequest = {
    base_rule_id: rule.base_rule_id,
    enabled: values.enabled,
    site_id: siteId,
  }
  if (rule.constraints.threshold_editable) {
    request.threshold_value = values.thresholdValue
  }
  if (rule.constraints.for_times_editable) {
    request.for_times = Number(values.forTimes)
  }
  return request
}

export function hasAlertRuleChanges(request: AlertRuleUpdateRequest): boolean {
  return Object.keys(request).length > 0
}
