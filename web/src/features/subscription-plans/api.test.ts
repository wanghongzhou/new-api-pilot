import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'

import {
  getSiteSubscriptionPlanStatistics,
  getSubscriptionPlanStatistics,
  listSiteSubscriptionPlans,
  listSubscriptionPlans,
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
      request_id: 'req_subscription_plan_frontend',
      success: true,
    },
    headers: {},
    status: 200,
    statusText: 'OK',
  }
}

describe('subscription plan pilot API contract', () => {
  test('uses exactly list and statistics global routes with frozen filters', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      return response(config)
    }) as AxiosAdapter
    const values = {
      enabled: false,
      keyword: 'safe-plan',
      p: 2,
      page_size: 20,
      site_ids: [parseIdString('9007199254740993')],
      states: ['missing' as const],
    }

    await listSubscriptionPlans(values)
    await getSubscriptionPlanStatistics(values)

    expect(requests.map((request) => request.url)).toEqual([
      '/api/subscription-plans',
      '/api/subscription-plans/statistics',
    ])
    for (const request of requests) {
      const params = request.params as URLSearchParams
      expect(params.get('enabled')).toBe('false')
      expect(params.get('keyword')).toBe('safe-plan')
      expect(params.getAll('site_ids')).toEqual(['9007199254740993'])
      expect(params.getAll('states')).toEqual(['missing'])
    }
  })

  test('uses only two forced site routes and strips global site ids', async () => {
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

    await listSiteSubscriptionPlans(siteId, values)
    await getSiteSubscriptionPlanStatistics(siteId, values)

    expect(requests.map((request) => request.url)).toEqual([
      '/api/sites/9007199254740993/subscription-plans',
      '/api/sites/9007199254740993/subscription-plans/statistics',
    ])
    for (const request of requests) {
      expect((request.params as URLSearchParams).has('site_ids')).toBe(false)
    }
  })
})
