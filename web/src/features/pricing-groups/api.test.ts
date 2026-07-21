import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'

import {
  getPricingCatalogStatistics,
  getSitePricingCatalogStatistics,
  listPricingCatalog,
  listPricingGroups,
  listSitePricingCatalog,
  listSitePricingGroups,
} from './api'

const originalAdapter = api.defaults.adapter
afterEach(() => {
  api.defaults.adapter = originalAdapter
})

describe('pricing/group API contract', () => {
  test('uses frozen global routes and filters', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return {
        config,
        data: {
          code: '',
          data: {},
          message: '',
          request_id: 'req',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter
    const values = {
      group: 'vip',
      keyword: 'gpt',
      p: 2,
      page_size: 20,
      site_ids: [parseIdString('9007199254740993')],
      states: ['missing' as const],
    }
    await listPricingCatalog(values)
    await getPricingCatalogStatistics(values)
    await listPricingGroups(values)
    expect(requests.map((request) => request.url)).toEqual([
      '/api/pricing-catalog',
      '/api/pricing-catalog/statistics',
      '/api/group-catalog',
    ])
    for (const request of requests) {
      const params = request.params as URLSearchParams
      expect(params.getAll('site_ids')).toEqual(['9007199254740993'])
      expect(params.get('group')).toBe('vip')
      expect(params.get('keyword')).toBe('gpt')
    }
  })

  test('uses forced site routes without site_ids', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return {
        config,
        data: {
          code: '',
          data: {},
          message: '',
          request_id: 'req',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter
    const siteId = parseIdString('9007199254740993')
    const values = { p: 1, page_size: 20, site_ids: [parseIdString('9')] }
    await listSitePricingCatalog(siteId, values)
    await getSitePricingCatalogStatistics(siteId, values)
    await listSitePricingGroups(siteId, values)
    expect(requests.map((request) => request.url)).toEqual([
      '/api/sites/9007199254740993/pricing-catalog',
      '/api/sites/9007199254740993/pricing-catalog/statistics',
      '/api/sites/9007199254740993/group-catalog',
    ])
    for (const request of requests) {
      expect((request.params as URLSearchParams).has('site_ids')).toBe(false)
    }
  })
})
