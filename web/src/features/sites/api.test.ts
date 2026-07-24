import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'

import {
  authorizeSite,
  getSitePerformance,
  getSiteStatistics,
  listSites,
  recheckSiteCapabilities,
} from './api'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

describe('site API', () => {
  test('omits request params when the site list has no filters', async () => {
    api.defaults.adapter = async (config) => {
      expect(config.method).toBe('get')
      expect(config.url).toBe('/api/sites')
      expect(config.params).toBeUndefined()
      return {
        config,
        data: {
          code: '',
          data: { items: [], page: 1, page_size: 20, total: 0 },
          message: '',
          request_id: 'req_sites',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }

    await listSites({})
  })

  test('keeps authorization secrets in the POST body only', async () => {
    const siteId = parseIdString('9007199254740993')
    const secret = 'root-access-token-secret'
    const adapter: AxiosAdapter = async (config) => {
      expect(config.method).toBe('post')
      expect(config.url).toBe(`/api/sites/${siteId}/authorize`)
      expect(config.url).not.toContain(secret)
      expect(config.params).toBeUndefined()
      expect(config.timeout).toBe(30_000)
      expect(String(config.data)).toContain(secret)
      return {
        config,
        data: {
          code: '',
          data: {
            backfill_run_id: null,
            capabilities: [],
            data_export_enabled: true,
            first_user_proof: {
              earliest_created_at: 1,
              min_user_id: '1',
              passed: true,
              snapshot_total: 1,
            },
            flow_data_validation: 'passed',
            root_created_at: 1,
            root_user_id: '1',
            statistics_start_at: 0,
            system_name: 'New API',
            version: 'v1',
          },
          message: '',
          request_id: 'req_authorize',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    api.defaults.adapter = adapter

    await authorizeSite(siteId, {
      access_token: secret,
      mode: 'existing_token',
      root_user_id: parseIdString('1'),
    })
  })

  test('keeps capability verification within the shared timeout', async () => {
    const siteId = parseIdString('9007199254740993')
    api.defaults.adapter = async (config) => {
      expect(config.method).toBe('post')
      expect(config.url).toBe(`/api/sites/${siteId}/recheck-capabilities`)
      expect(config.timeout).toBe(30_000)
      return {
        config,
        data: {
          code: '',
          data: {},
          message: '',
          request_id: 'req_recheck',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }

    await recheckSiteCapabilities(siteId)
  })

  test('serializes multi-status filters as repeated query keys', async () => {
    api.defaults.adapter = async (config) => {
      const params = config.params as URLSearchParams
      expect(params.getAll('online_status')).toEqual(['offline', 'unknown'])
      expect(params.get('keyword')).toBe('华东')
      return {
        config,
        data: {
          code: '',
          data: { items: [], page: 1, page_size: 20, total: 0 },
          message: '',
          request_id: 'req_sites',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }

    await listSites({
      keyword: '华东',
      online_status: ['offline', 'unknown'],
    })
  })

  test('requests a site performance summary for the selected time range', async () => {
    const siteId = parseIdString('9007199254740993')
    api.defaults.adapter = async (config) => {
      expect(config.method).toBe('get')
      expect(config.url).toBe(`/api/sites/${siteId}/performance`)
      expect(config.params).toEqual({ hours: 168 })
      return {
        config,
        data: {
          code: '',
          data: {
            avg_latency_ms: 120,
            avg_tps: 40,
            data_status: 'complete',
            hours: 168,
            models: [],
            request_count: '20',
            sampled_at: 100,
            success_rate: 99.5,
          },
          message: '',
          request_id: 'req_site_performance',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }

    await getSitePerformance(siteId, 168)
  })

  test('requests the embedded site dashboard statistics for its selected range', async () => {
    const siteId = parseIdString('9007199254740993')
    api.defaults.adapter = async (config) => {
      expect(config.method).toBe('get')
      expect(config.url).toBe(`/api/sites/${siteId}/stats`)
      expect(Object.fromEntries(config.params as URLSearchParams)).toEqual({
        end_timestamp: '1783875600',
        granularity: 'day',
        p: '1',
        page_size: '20',
        sort_by: 'bucket_start',
        sort_order: 'desc',
        start_timestamp: '1783789200',
      })
      return {
        config,
        data: {
          code: '',
          data: {},
          message: '',
          request_id: 'req_site_statistics',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }

    await getSiteStatistics(siteId, {
      end_timestamp: 1_783_875_600,
      granularity: 'day',
      p: 1,
      page_size: 20,
      sort_by: 'bucket_start',
      sort_order: 'desc',
      start_timestamp: 1_783_789_200,
    })
  })
})
