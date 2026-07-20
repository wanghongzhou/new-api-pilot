import { requestApiData } from '@/lib/api'

import type {
  NotificationTestResult,
  SettingGroup,
  SettingPatchRequest,
} from './types'

export function getSettings(): Promise<SettingGroup[]> {
  return requestApiData<SettingGroup[]>({
    method: 'get',
    url: '/api/settings',
  })
}

export function updateSettings(
  request: SettingPatchRequest
): Promise<SettingGroup[]> {
  return requestApiData<SettingGroup[]>({
    data: request,
    method: 'put',
    url: '/api/settings',
  })
}

export function testDingTalkNotification(): Promise<NotificationTestResult> {
  return requestApiData<NotificationTestResult>({
    method: 'post',
    url: '/api/notifications/dingtalk/test',
  })
}
