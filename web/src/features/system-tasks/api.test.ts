import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'

import {
  getSiteSystemTaskStatistics,
  getSystemTaskStatistics,
  listSiteSystemTasks,
  listSystemTasks,
} from './api'

const originalAdapter = api.defaults.adapter
afterEach(() => {
  api.defaults.adapter = originalAdapter
})

describe('system task read-only API contract', () => {
  test('uses only global GET list/statistics routes and frozen filters', async () => {
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
      created_end: 200,
      created_start: 100,
      error_present: false,
      p: 2,
      page_size: 20,
      site_ids: [parseIdString('9007199254740993')],
      statuses: ['failed' as const],
      types: ['log_cleanup' as const],
    }
    await listSystemTasks(values)
    await getSystemTaskStatistics(values)
    expect(
      requests.map((request) => `${request.method}:${request.url}`)
    ).toEqual(['get:/api/system-tasks', 'get:/api/system-tasks/statistics'])
    for (const request of requests) {
      const params = request.params as URLSearchParams
      expect(params.getAll('site_ids')).toEqual(['9007199254740993'])
      expect(params.get('error_present')).toBe('false')
      expect(params.get('created_start')).toBe('100')
    }
  })
  test('uses forced site GET routes and strips site_ids', async () => {
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
    const site = parseIdString('9007199254740993')
    const values = { p: 1, page_size: 20, site_ids: [parseIdString('9')] }
    await listSiteSystemTasks(site, values)
    await getSiteSystemTaskStatistics(site, values)
    expect(
      requests.map((request) => `${request.method}:${request.url}`)
    ).toEqual([
      'get:/api/sites/9007199254740993/system-tasks',
      'get:/api/sites/9007199254740993/system-tasks/statistics',
    ])
    for (const request of requests)
      expect((request.params as URLSearchParams).has('site_ids')).toBe(false)
  })
})
