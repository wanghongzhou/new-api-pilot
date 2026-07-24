import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'

import {
  createAlertRuleOverride,
  deleteAlertRuleOverride,
  getAlert,
  getAlertSummary,
  listAlertRules,
  listAlerts,
  updateAlertRule,
} from './api'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

describe('alert API contract', () => {
  test('uses only the documented Viewer read endpoints with exact query keys', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return {
        config,
        data: {
          code: '',
          data:
            config.url === '/api/alert-rules'
              ? { items: [], page: 1, page_size: 20, total: 0 }
              : {},
          message: '',
          request_id: 'req_alerts',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter

    const bigintId = parseIdString('9007199254740993')
    await getAlertSummary()
    await listAlerts({
      end_timestamp: 1_784_000_000,
      level: ['critical', 'warning'],
      p: 2,
      page_size: 20,
      site_id: bigintId,
      sort_by: 'last_fired_at',
      sort_order: 'desc',
      start_timestamp: 1_783_900_000,
      status: ['firing', 'pending'],
      target_type: ['site', 'account'],
    })
    await getAlert(bigintId)
    await listAlertRules({
      category: ['instance', 'channel'],
      enabled: true,
      level: ['critical', 'warning'],
      p: 1,
      page_size: 20,
      scope_type: 'global',
      sort_by: 'category',
      sort_order: 'asc',
    })
    await listAlertRules({
      inherited: false,
      p: 2,
      page_size: 10,
      scope_id: bigintId,
      scope_type: 'site',
      sort_by: 'updated_at',
      sort_order: 'desc',
    })

    expect(requests.map(({ method, url }) => [method, url])).toEqual([
      ['get', '/api/alerts/summary'],
      ['get', '/api/alerts'],
      ['get', '/api/alerts/9007199254740993'],
      ['get', '/api/alert-rules'],
      ['get', '/api/alert-rules'],
    ])
    const alertParams = requests[1]?.params as URLSearchParams
    expect(Object.fromEntries(alertParams)).toEqual({
      end_timestamp: '1784000000',
      level: 'warning',
      p: '2',
      page_size: '20',
      site_id: '9007199254740993',
      sort_by: 'last_fired_at',
      sort_order: 'desc',
      start_timestamp: '1783900000',
      status: 'pending',
      target_type: 'account',
    })
    expect(alertParams.getAll('level')).toEqual(['critical', 'warning'])
    expect(alertParams.getAll('status')).toEqual(['firing', 'pending'])
    expect(alertParams.getAll('target_type')).toEqual(['site', 'account'])
    const globalRuleParams = requests[3]?.params as URLSearchParams
    expect(globalRuleParams.getAll('category')).toEqual(['instance', 'channel'])
    expect(globalRuleParams.getAll('level')).toEqual(['critical', 'warning'])
    expect(Object.fromEntries(globalRuleParams)).toEqual({
      category: 'channel',
      enabled: 'true',
      level: 'warning',
      p: '1',
      page_size: '20',
      scope_type: 'global',
      sort_by: 'category',
      sort_order: 'asc',
    })
    expect(Object.fromEntries(requests[4]?.params as URLSearchParams)).toEqual({
      inherited: 'false',
      p: '2',
      page_size: '10',
      scope_id: '9007199254740993',
      scope_type: 'site',
      sort_by: 'updated_at',
      sort_order: 'desc',
    })
  })

  test('uses the three documented Admin rule mutations and preserves string IDs', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return {
        config,
        data: {
          code: '',
          data: config.method === 'delete' ? null : {},
          message: '',
          request_id: 'req_alert_rules',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter

    const ruleId = parseIdString('9007199254740993')
    const siteId = parseIdString('9007199254740995')
    await updateAlertRule(ruleId, {
      enabled: false,
      for_times: 3,
      threshold_value: '79.50',
    })
    await createAlertRuleOverride({
      base_rule_id: ruleId,
      enabled: true,
      for_times: 4,
      site_id: siteId,
      threshold_value: '81.25',
    })
    await deleteAlertRuleOverride(ruleId)

    expect(requests.map(({ method, url }) => [method, url])).toEqual([
      ['put', '/api/alert-rules/9007199254740993'],
      ['post', '/api/alert-rules/overrides'],
      ['delete', '/api/alert-rules/9007199254740993'],
    ])
    expect(JSON.parse(String(requests[0]?.data))).toEqual({
      enabled: false,
      for_times: 3,
      threshold_value: '79.50',
    })
    expect(JSON.parse(String(requests[1]?.data))).toEqual({
      base_rule_id: '9007199254740993',
      enabled: true,
      for_times: 4,
      site_id: '9007199254740995',
      threshold_value: '81.25',
    })
    expect(requests[2]?.data).toBeUndefined()
  })
})
