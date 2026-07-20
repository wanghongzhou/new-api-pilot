import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'

import { listLogs, listSiteLogs } from './api'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

describe('log API contract', () => {
  test('serializes every frozen global filter without bigint coercion', async () => {
    api.defaults.adapter = (async (config) => {
      expect(config.url).toBe('/api/logs')
      const params = config.params as URLSearchParams
      expect(params.getAll('site_ids')).toEqual([
        '9007199254740993',
        '9007199254740995',
      ])
      expect(params.get('channel_id')).toBe('9007199254740997')
      expect(params.get('type')).toBe('5')
      expect(params.get('username')).toBe('alice')
      expect(params.get('model_name')).toBe('gpt-4.1')
      expect(params.get('token_name')).toBe('production')
      expect(params.get('group')).toBe('vip')
      expect(params.get('request_id')).toBe('req-local')
      expect(params.get('upstream_request_id')).toBe('req-upstream')
      return {
        config,
        data: {
          code: '',
          data: {},
          message: '',
          request_id: 'req_logs',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter

    await listLogs({
      channel_id: parseNonNegativeIdString('9007199254740997'),
      end_timestamp: 1_784_262_400,
      group: 'vip',
      model_name: 'gpt-4.1',
      p: 2,
      page_size: 50,
      request_id: 'req-local',
      site_ids: [
        parseIdString('9007199254740993'),
        parseIdString('9007199254740995'),
      ],
      start_timestamp: 1_784_176_000,
      token_name: 'production',
      type: 5,
      upstream_request_id: 'req-upstream',
      username: 'alice',
    })
  })

  test('uses the forced site endpoint and excludes site_ids', async () => {
    api.defaults.adapter = (async (config) => {
      expect(config.url).toBe('/api/sites/9007199254740993/logs')
      expect((config.params as URLSearchParams).has('site_ids')).toBe(false)
      return {
        config,
        data: {
          code: '',
          data: {},
          message: '',
          request_id: 'req_site_logs',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter
    await listSiteLogs(parseIdString('9007199254740993'), {
      end_timestamp: 1_784_262_400,
      p: 1,
      page_size: 20,
      site_ids: [parseIdString('9007199254740995')],
      start_timestamp: 1_784_176_000,
    })
  })
})
