import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'

import {
  getPerformanceHistoryStatistics,
  getSitePerformanceHistoryStatistics,
  listPerformanceHistory,
  listSitePerformanceHistory,
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
      request_id: 'req_performance_history',
      success: true,
    },
    headers: {},
    status: 200,
    statusText: 'OK',
  }
}

describe('performance history API contract', () => {
  test('serializes repeated site/model/group filters exactly', async () => {
    api.defaults.adapter = (async (config) => {
      const params = config.params as URLSearchParams
      expect(config.url).toBe('/api/performance-history')
      expect(params.getAll('site_ids')).toEqual(['9007199254740993'])
      expect(params.getAll('model_names')).toEqual(['GPT-5', 'gpt-5'])
      expect(params.getAll('groups')).toEqual(['default', 'vip'])
      return response(config)
    }) as AxiosAdapter
    await listPerformanceHistory({
      end_timestamp: 1_784_348_800,
      groups: ['default', 'vip'],
      model_names: ['GPT-5', 'gpt-5'],
      p: 1,
      page_size: 20,
      site_ids: [parseIdString('9007199254740993')],
      start_timestamp: 1_784_262_400,
    })
  })

  test('uses forced site endpoints and removes global site filters', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return response(config)
    }) as AxiosAdapter
    const siteId = parseIdString('9007199254740993')
    const params = {
      end_timestamp: 1_784_348_800,
      p: 1,
      page_size: 20,
      site_ids: [parseIdString('9007199254740995')],
      start_timestamp: 1_784_262_400,
    }
    await listSitePerformanceHistory(siteId, params)
    await getSitePerformanceHistoryStatistics(siteId, params)
    await getPerformanceHistoryStatistics(params)
    expect(requests.map((request) => request.url)).toEqual([
      '/api/sites/9007199254740993/performance-history',
      '/api/sites/9007199254740993/performance-history/statistics',
      '/api/performance-history/statistics',
    ])
    for (const request of requests.slice(0, 2)) {
      expect((request.params as URLSearchParams).has('site_ids')).toBe(false)
    }
  })
})
