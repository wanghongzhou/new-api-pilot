import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'

import {
  getSiteUpstreamTaskStatistics,
  getUpstreamTaskStatistics,
  listSiteUpstreamTasks,
  listUpstreamTasks,
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
      request_id: 'req_upstream_task_frontend',
      success: true,
    },
    headers: {},
    status: 200,
    statusText: 'OK',
  }
}

describe('upstream task API contract', () => {
  test('serializes every frozen filter without bigint coercion', async () => {
    api.defaults.adapter = (async (config) => {
      const params = config.params as URLSearchParams
      expect(config.url).toBe('/api/upstream-tasks')
      expect(params.getAll('site_ids')).toEqual(['9007199254740993'])
      expect(params.getAll('statuses')).toEqual(['IN_PROGRESS', 'SUCCESS'])
      expect(params.get('remote_id')).toBe('9007199254740995')
      expect(params.get('remote_user_id')).toBe('0')
      expect(params.get('remote_channel_id')).toBe('9007199254740997')
      expect(params.get('task_id')).toBe('task-safe')
      expect(params.has('start_timestamp')).toBe(false)
      expect(params.has('end_timestamp')).toBe(false)
      return response(config)
    }) as AxiosAdapter
    await listUpstreamTasks({
      p: 2,
      page_size: 20,
      remote_channel_id: parseNonNegativeIdString('9007199254740997'),
      remote_id: parseIdString('9007199254740995'),
      remote_user_id: parseNonNegativeIdString('0'),
      site_ids: [parseIdString('9007199254740993')],
      statuses: ['IN_PROGRESS', 'SUCCESS'],
      task_id: 'task-safe',
    })
  })

  test('uses forced site endpoints and strips site ids for list and statistics', async () => {
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
    await listSiteUpstreamTasks(siteId, values)
    await getSiteUpstreamTaskStatistics(siteId, values)
    await getUpstreamTaskStatistics(values)
    expect(requests.map((request) => request.url)).toEqual([
      '/api/sites/9007199254740993/upstream-tasks',
      '/api/sites/9007199254740993/upstream-tasks/statistics',
      '/api/upstream-tasks/statistics',
    ])
    for (const request of requests.slice(0, 2)) {
      expect((request.params as URLSearchParams).has('site_ids')).toBe(false)
    }
  })
})
