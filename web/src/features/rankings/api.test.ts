import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'

import { getRankings, getSiteRankings } from './api'

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
      request_id: 'req_local_rankings_frontend',
      success: true,
    },
    headers: {},
    status: 200,
    statusText: 'OK',
  }
}

describe('local rankings pilot API contract', () => {
  test('uses only pilot global routes and serializes period and bigint site ids', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return response(config)
    }) as AxiosAdapter
    const values = {
      period: 'month' as const,
      site_ids: [parseIdString('9007199254740993')],
    }

    await getRankings('models', values)
    await getRankings('vendors', values)

    expect(requests.map((request) => request.url)).toEqual([
      '/api/rankings/models',
      '/api/rankings/vendors',
    ])
    for (const request of requests) {
      const params = request.params as URLSearchParams
      expect(params.get('period')).toBe('month')
      expect(params.getAll('site_ids')).toEqual(['9007199254740993'])
    }
  })

  test('uses forced pilot site routes and strips caller-provided site ids', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return response(config)
    }) as AxiosAdapter
    const siteId = parseIdString('9007199254740993')
    const values = {
      period: 'week' as const,
      site_ids: [parseIdString('9007199254740995')],
    }

    await getSiteRankings(siteId, 'models', values)
    await getSiteRankings(siteId, 'vendors', values)

    expect(requests.map((request) => request.url)).toEqual([
      '/api/sites/9007199254740993/rankings/models',
      '/api/sites/9007199254740993/rankings/vendors',
    ])
    for (const request of requests) {
      const params = request.params as URLSearchParams
      expect(params.get('period')).toBe('week')
      expect(params.has('site_ids')).toBe(false)
    }
  })
})
