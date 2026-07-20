import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import {
  parseDecimalString,
  parseIdString,
  parseMetricString,
} from '@/lib/api-types'

import {
  getChannelInventoryStatistics,
  getSiteChannelInventoryStatistics,
  listChannelInventory,
  listSiteChannelInventory,
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
      request_id: 'req_channel_inventory',
      success: true,
    },
    headers: {},
    status: 200,
    statusText: 'OK',
  }
}

describe('channel inventory API contract', () => {
  test('serializes exact decimals and bigint filters without coercion', async () => {
    api.defaults.adapter = (async (config) => {
      expect(config.url).toBe('/api/channel-inventory')
      const params = config.params as URLSearchParams
      expect(params.getAll('site_ids')).toEqual(['9007199254740993'])
      expect(params.getAll('types')).toEqual(['1', '8'])
      expect(params.getAll('statuses')).toEqual(['1', '3'])
      expect(params.getAll('groups')).toEqual(['vip'])
      expect(params.getAll('tags')).toEqual(['primary'])
      expect(params.getAll('states')).toEqual(['normal', 'missing'])
      expect(params.get('max_balance')).toBe('9007199254740993.1234567890')
      expect(params.get('max_response_time_ms')).toBe('9223372036854775807')
      expect(params.has('key')).toBe(false)
      return response(config)
    }) as AxiosAdapter
    await listChannelInventory({
      groups: ['vip'],
      max_balance: parseDecimalString('9007199254740993.1234567890'),
      max_response_time_ms: parseMetricString('9223372036854775807'),
      p: 2,
      page_size: 50,
      site_ids: [parseIdString('9007199254740993')],
      states: ['normal', 'missing'],
      statuses: [1, 3],
      tags: ['primary'],
      types: [1, 8],
    })
  })

  test('uses forced site endpoints and strips site_ids from list and statistics', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return response(config)
    }) as AxiosAdapter
    const siteId = parseIdString('9007199254740993')
    await listSiteChannelInventory(siteId, {
      p: 1,
      page_size: 20,
      site_ids: [parseIdString('9007199254740995')],
    })
    await getSiteChannelInventoryStatistics(siteId, {
      end_timestamp: 1_784_348_800,
      site_ids: [parseIdString('9007199254740995')],
      start_timestamp: 1_784_262_400,
    })
    await getChannelInventoryStatistics({
      end_timestamp: 1_784_348_800,
      site_ids: [siteId],
      start_timestamp: 1_784_262_400,
    })
    expect(requests.map((request) => request.url)).toEqual([
      '/api/sites/9007199254740993/channel-inventory',
      '/api/sites/9007199254740993/channel-inventory/statistics',
      '/api/channel-inventory/statistics',
    ])
    for (const request of requests.slice(0, 2)) {
      expect((request.params as URLSearchParams).has('site_ids')).toBe(false)
    }
    const globalParams = requests.at(2)?.params as URLSearchParams | undefined
    if (!globalParams) throw new Error('expected global statistics request')
    expect(globalParams.getAll('site_ids')).toEqual(['9007199254740993'])
  })
})
