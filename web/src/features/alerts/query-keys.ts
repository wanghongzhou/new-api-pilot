import type { IdString } from '@/lib/api-types'

import type { AlertListParams, AlertRuleListParams } from './types'

function stableAlertParams(params: AlertListParams | AlertRuleListParams) {
  return Object.fromEntries(
    Object.entries(params)
      .filter(([, value]) => value !== undefined)
      .map(([key, value]) => [
        key,
        Array.isArray(value) ? [...value].sort() : value,
      ])
      .sort(([left], [right]) => left.localeCompare(right))
  )
}

export const alertKeys = {
  all: ['alerts'] as const,
  summary: () => ['alerts', 'summary'] as const,
  lists: () => ['alerts', 'list'] as const,
  list: (params: AlertListParams) =>
    ['alerts', 'list', stableAlertParams(params)] as const,
  detail: (id: IdString) => ['alerts', 'detail', id] as const,
  rules: (params: AlertRuleListParams) =>
    ['alerts', 'rules', stableAlertParams(params)] as const,
}
