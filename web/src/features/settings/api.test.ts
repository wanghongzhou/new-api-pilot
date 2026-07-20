import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'

import { getSettings, testDingTalkNotification, updateSettings } from './api'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

describe('settings API contract', () => {
  test('uses Viewer GET and atomic PUT endpoints with exact scalar JSON types', async () => {
    const requests: Array<{ data: unknown; method?: string; url?: string }> = []
    api.defaults.adapter = (async (config) => {
      requests.push({
        data: config.data,
        method: config.method,
        url: config.url,
      })
      return {
        config,
        data: {
          code: '',
          data: [],
          message: '',
          request_id: 'req_settings',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter

    await getSettings()
    await updateSettings({
      items: [
        { key: 'collector.usage_delay_minutes', value: 5 },
        { key: 'export.max_file_bytes', value: '9007199254740993' },
        { key: 'rate.fallback_usd_exchange_rate', value: '7.3000' },
      ],
    })

    expect(requests.map(({ method, url }) => [method, url])).toEqual([
      ['get', '/api/settings'],
      ['put', '/api/settings'],
    ])
    const body = JSON.parse(String(requests[1]?.data)) as {
      items: Array<{ value: unknown }>
    }
    expect(body.items.map((item) => typeof item.value)).toEqual([
      'number',
      'string',
      'string',
    ])
  })

  test('uses the saved-config DingTalk test command without a request body', async () => {
    api.defaults.adapter = (async (config) => {
      expect(config.method).toBe('post')
      expect(config.url).toBe('/api/notifications/dingtalk/test')
      expect(config.data).toBeUndefined()
      return {
        config,
        data: {
          code: '',
          data: {
            delivery_id: '9007199254740993',
            message: {
              code: 'NOTIFICATION_TEST_SUCCEEDED',
              params: { delivery_id: '9007199254740993' },
              technical_detail: '',
            },
            response_code: 200,
            status: 'success',
          },
          message: '',
          request_id: 'req_notification_test',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter

    const result = await testDingTalkNotification()
    expect(result.message.code).toBe('NOTIFICATION_TEST_SUCCEEDED')
  })
})
