import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'

import {
  getDashboardHealth,
  getDashboardSummary,
  getDashboardTop,
  getDashboardTrend,
} from './api'
import { dashboardKeys } from './query-keys'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

describe('dashboard API contract', () => {
  test('uses exactly the four documented read endpoints', async () => {
    const requests: Array<{ params: unknown; url?: string }> = []
    api.defaults.adapter = (async (config) => {
      requests.push({ params: config.params, url: config.url })
      return {
        config,
        data: {
          code: '',
          data: config.url?.endsWith('/summary') ? {} : [],
          message: '',
          request_id: 'req_dashboard',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter

    await Promise.all([
      getDashboardSummary(),
      getDashboardTrend(30),
      getDashboardTop('customer', 'request_count', 5),
      getDashboardHealth(),
    ])

    expect(requests.map((request) => request.url)).toEqual([
      '/api/dashboard/summary',
      '/api/dashboard/trend',
      '/api/dashboard/top',
      '/api/dashboard/health',
    ])
    expect(requests[1]?.params).toEqual({ days: 30 })
    expect(requests[2]?.params).toEqual({
      limit: 5,
      metric: 'request_count',
      type: 'customer',
    })
  })

  test('keeps each ranking type in one typed key slot including its limit', () => {
    expect(dashboardKeys.top('customer', 'quota', 10)).toEqual([
      'dashboard',
      'top',
      { limit: 10, metric: 'quota', type: 'customer' },
    ])
    expect(
      (['site', 'customer', 'model', 'channel'] as const).map((type) =>
        dashboardKeys.top(type, 'request_count', 5)
      )
    ).toEqual([
      dashboardKeys.top('site', 'request_count', 5),
      dashboardKeys.top('customer', 'request_count', 5),
      dashboardKeys.top('model', 'request_count', 5),
      dashboardKeys.top('channel', 'request_count', 5),
    ])
  })

  test('rejects ranking limits outside the documented integer range', () => {
    expect(() => getDashboardTop('site', 'quota', 0)).toThrow(RangeError)
    expect(() => getDashboardTop('channel', 'quota', 21)).toThrow(RangeError)
    expect(() => getDashboardTop('model', 'quota', 1.5)).toThrow(RangeError)
  })
})
