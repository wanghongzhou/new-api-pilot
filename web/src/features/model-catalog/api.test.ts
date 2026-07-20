import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'

import {
  getModelCoverage,
  getSiteModelCoverage,
  listMissingModels,
  listModelCatalog,
  listSiteMissingModels,
  listSiteModelCatalog,
} from './api'
import type { ModelCatalogQueryParams } from './types'

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
      request_id: 'req_model_catalog_frontend',
      success: true,
    },
    headers: {},
    status: 200,
    statusText: 'OK',
  }
}

describe('model catalog API contract', () => {
  test('serializes all frozen global filters for all three routes', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return response(config)
    }) as AxiosAdapter
    const values: ModelCatalogQueryParams = {
      keyword: 'safe-model',
      p: 2,
      page_size: 20,
      site_ids: [parseIdString('9007199254740993')],
      statuses: [0, 1],
      sync_official: [1],
      vendor_id: parseNonNegativeIdString('0'),
    }
    await listModelCatalog(values)
    await getModelCoverage(values)
    await listMissingModels(values)
    expect(requests.map((request) => request.url)).toEqual([
      '/api/model-catalog',
      '/api/model-catalog/coverage',
      '/api/model-catalog/missing',
    ])
    for (const request of requests) {
      const params = request.params as URLSearchParams
      expect(params.getAll('site_ids')).toEqual(['9007199254740993'])
      expect(params.getAll('statuses')).toEqual(['0', '1'])
      expect(params.getAll('sync_official')).toEqual(['1'])
      expect(params.get('vendor_id')).toBe('0')
      expect(params.get('keyword')).toBe('safe-model')
    }
  })

  test('uses forced site routes and strips site ids', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return response(config)
    }) as AxiosAdapter
    const siteId = parseIdString('9007199254740993')
    const values = {
      p: 1,
      page_size: 20,
      site_ids: [parseIdString('9007199254740995')],
    }
    await listSiteModelCatalog(siteId, values)
    await getSiteModelCoverage(siteId, values)
    await listSiteMissingModels(siteId, values)
    expect(requests.map((request) => request.url)).toEqual([
      '/api/sites/9007199254740993/model-catalog',
      '/api/sites/9007199254740993/model-catalog/coverage',
      '/api/sites/9007199254740993/model-catalog/missing',
    ])
    for (const request of requests) {
      expect((request.params as URLSearchParams).has('site_ids')).toBe(false)
    }
  })
})
