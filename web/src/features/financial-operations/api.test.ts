import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'

import {
  getRedemptionStatistics,
  getSiteTopupStatistics,
  listRedemptions,
  listSiteTopups,
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
      request_id: 'req_finance_frontend',
      success: true,
    },
    headers: {},
    status: 200,
    statusText: 'OK',
  }
}

describe('financial operations API contract', () => {
  test('serializes all safe repeated filters and preserves zero user ids', async () => {
    api.defaults.adapter = (async (config) => {
      const params = config.params as URLSearchParams
      expect(config.url).toBe('/api/redemptions')
      expect(params.getAll('site_ids')).toEqual(['9007199254740993'])
      expect(params.getAll('statuses')).toEqual(['1', '2'])
      expect(params.getAll('states')).toEqual(['normal', 'missing'])
      expect(params.get('remote_user_id')).toBe('0')
      expect(params.get('keyword')).toBe('batch-safe')
      return response(config)
    }) as AxiosAdapter
    await listRedemptions({
      end_timestamp: 1_784_348_800,
      keyword: 'batch-safe',
      p: 1,
      page_size: 20,
      remote_user_id: parseNonNegativeIdString('0'),
      site_ids: [parseIdString('9007199254740993')],
      start_timestamp: 1_784_262_400,
      states: ['normal', 'missing'],
      statuses: ['1', '2'],
    })
  })

  test('uses forced site endpoints and strips site ids', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return response(config)
    }) as AxiosAdapter
    const siteId = parseIdString('9007199254740993')
    const params = {
      p: 1,
      page_size: 20,
      site_ids: [parseIdString('9007199254740995')],
    }
    await listSiteTopups(siteId, params)
    await getSiteTopupStatistics(siteId, params)
    await getRedemptionStatistics(params)
    expect(requests.map((request) => request.url)).toEqual([
      '/api/sites/9007199254740993/topups',
      '/api/sites/9007199254740993/topups/statistics',
      '/api/redemptions/statistics',
    ])
    for (const request of requests.slice(0, 2)) {
      expect((request.params as URLSearchParams).has('site_ids')).toBe(false)
    }
  })
})
