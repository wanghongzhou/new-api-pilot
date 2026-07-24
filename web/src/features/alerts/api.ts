import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  AlertEventDetail,
  AlertEventPage,
  AlertListParams,
  AlertRuleItem,
  AlertRuleListParams,
  AlertRuleOverrideRequest,
  AlertRulePage,
  AlertRuleUpdateRequest,
  AlertSummary,
} from './types'

function alertSearchParams(
  values: AlertListParams | AlertRuleListParams
): URLSearchParams {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(values)) {
    if (value === undefined || value === '') continue
    if (Array.isArray(value)) {
      for (const item of value) params.append(key, item)
    } else {
      params.set(key, String(value))
    }
  }
  return params
}

export function getAlertSummary(): Promise<AlertSummary> {
  return requestApiData<AlertSummary>({
    method: 'get',
    url: '/api/alerts/summary',
  })
}

export function listAlerts(params: AlertListParams): Promise<AlertEventPage> {
  return requestApiData<AlertEventPage>({
    method: 'get',
    params: alertSearchParams(params),
    url: '/api/alerts',
  })
}

export function getAlert(id: IdString): Promise<AlertEventDetail> {
  return requestApiData<AlertEventDetail>({
    method: 'get',
    url: `/api/alerts/${id}`,
  })
}

export function listAlertRules(
  params: AlertRuleListParams
): Promise<AlertRulePage> {
  return requestApiData<AlertRulePage>({
    method: 'get',
    params: alertSearchParams(params),
    url: '/api/alert-rules',
  })
}

export function updateAlertRule(
  id: IdString,
  request: AlertRuleUpdateRequest
): Promise<AlertRuleItem> {
  return requestApiData<AlertRuleItem>({
    data: request,
    method: 'put',
    url: `/api/alert-rules/${id}`,
  })
}

export function createAlertRuleOverride(
  request: AlertRuleOverrideRequest
): Promise<AlertRuleItem> {
  return requestApiData<AlertRuleItem>({
    data: request,
    method: 'post',
    url: '/api/alert-rules/overrides',
  })
}

export function deleteAlertRuleOverride(id: IdString): Promise<null> {
  return requestApiData<null>({
    method: 'delete',
    url: `/api/alert-rules/${id}`,
  })
}
