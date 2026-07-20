import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString, parseMetricString } from '@/lib/api-types'

import {
  getSiteUserInventoryStatistics,
  getUserInventoryStatistics,
  listSiteUserInventory,
  listUserInventory,
} from './api'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

function response(config: Parameters<AxiosAdapter>[0]) {
  return {
    config,
    data: {
      code: '',
      data: {},
      message: '',
      request_id: 'req_inventory',
      success: true,
    },
    headers: {},
    status: 200,
    statusText: 'OK',
  }
}

describe('user inventory API contract', () => {
  test('serializes safe global list filters without bigint coercion', async () => {
    api.defaults.adapter = (async (config) => {
      expect(config.url).toBe('/api/user-inventory')
      const params = config.params as URLSearchParams
      expect(params.getAll('site_ids')).toEqual(['9007199254740993'])
      expect(params.getAll('roles')).toEqual(['1', '100'])
      expect(params.getAll('statuses')).toEqual(['1', '2'])
      expect(params.getAll('groups')).toEqual(['vip'])
      expect(params.getAll('states')).toEqual(['missing', 'identity_mismatch'])
      expect(params.get('min_balance')).toBe('-9223372036854775808')
      expect(params.get('max_balance')).toBe('9223372036854775807')
      expect(params.get('keyword')).toBe('alice')
      expect(params.get('remote_user_id')).toBe('9007199254740997')
      return response(config)
    }) as AxiosAdapter
    await listUserInventory({
      groups: ['vip'],
      keyword: 'alice',
      max_balance: parseMetricString('9223372036854775807'),
      min_balance: parseMetricString('-9223372036854775808'),
      p: 2,
      page_size: 50,
      remote_user_id: parseIdString('9007199254740997'),
      roles: [1, 100],
      site_ids: [parseIdString('9007199254740993')],
      states: ['missing', 'identity_mismatch'],
      statuses: [1, 2],
    })
  })

  test('uses forced site endpoints and strips site_ids from list and statistics', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return response(config)
    }) as AxiosAdapter
    const siteId = parseIdString('9007199254740993')
    await listSiteUserInventory(siteId, {
      p: 1,
      page_size: 20,
      site_ids: [parseIdString('9007199254740995')],
    })
    await getSiteUserInventoryStatistics(siteId, {
      end_timestamp: 1_784_262_400,
      site_ids: [parseIdString('9007199254740995')],
      start_timestamp: 1_784_176_000,
    })
    await getUserInventoryStatistics({
      end_timestamp: 1_784_262_400,
      site_ids: [siteId],
      start_timestamp: 1_784_176_000,
    })
    expect(requests.map((request) => request.url)).toEqual([
      '/api/sites/9007199254740993/user-inventory',
      '/api/sites/9007199254740993/user-inventory/statistics',
      '/api/user-inventory/statistics',
    ])
    const listParams = requests.at(0)?.params as URLSearchParams | undefined
    const siteStatsParams = requests.at(1)?.params as
      | URLSearchParams
      | undefined
    const globalStatsParams = requests.at(2)?.params as
      | URLSearchParams
      | undefined
    if (!listParams || !siteStatsParams || !globalStatsParams) {
      throw new Error('expected all inventory requests')
    }
    expect(listParams.has('site_ids')).toBe(false)
    expect(siteStatsParams.has('site_ids')).toBe(false)
    expect(globalStatsParams.getAll('site_ids')).toEqual(['9007199254740993'])
  })
})
