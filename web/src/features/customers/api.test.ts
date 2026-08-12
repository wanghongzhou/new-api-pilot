import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'

import {
  enableCustomer,
  getCustomerStatistics,
  listAllCustomers,
  listCustomers,
  updateCustomer,
} from './api'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

function successAdapter(
  assertRequest: (config: Parameters<AxiosAdapter>[0]) => void
): AxiosAdapter {
  return async (config) => {
    assertRequest(config)
    return {
      config,
      data: {
        code: '',
        data: {},
        message: '',
        request_id: 'req_customer',
        success: true,
      },
      headers: {},
      status: 200,
      statusText: 'OK',
    }
  }
}

describe('customer API', () => {
  test('serializes status filters as repeated query keys', async () => {
    api.defaults.adapter = successAdapter((config) => {
      const params = config.params as URLSearchParams
      expect(config.url).toBe('/api/customers')
      expect(params.getAll('status')).toEqual(['using', 'disabled'])
      expect(params.get('keyword')).toBe('示例')
    })
    await listCustomers({ keyword: '示例', status: ['using', 'disabled'] })
  })

  test('uses the customer convenience statistics endpoint with the exact range', async () => {
    const customerId = parseIdString('9007199254740993')
    api.defaults.adapter = successAdapter((config) => {
      const params = config.params as URLSearchParams
      expect(config.url).toBe(`/api/customers/${customerId}/stats`)
      expect(Object.fromEntries(params)).toEqual({
        end_timestamp: '1783875600',
        granularity: 'hour',
        p: '2',
        page_size: '20',
        sort_by: 'bucket_start',
        sort_order: 'asc',
        start_timestamp: '1783789200',
      })
    })
    await getCustomerStatistics(customerId, {
      end_timestamp: 1_783_875_600,
      granularity: 'hour',
      p: 2,
      page_size: 20,
      sort_by: 'bucket_start',
      sort_order: 'asc',
      start_timestamp: 1_783_789_200,
    })
  })

  test('updates only editable profile fields and sends enable with no body', async () => {
    const customerId = parseIdString('7')
    let call = 0
    api.defaults.adapter = successAdapter((config) => {
      call += 1
      if (call === 1) {
        expect(config.method).toBe('put')
        expect(JSON.parse(String(config.data))).toEqual({
          contact: '张经理',
          name: '示例客户',
          remark: '重点客户',
          status: 'using',
        })
      } else {
        expect(config.method).toBe('post')
        expect(config.url).toBe('/api/customers/7/enable')
        expect(config.data).toBeUndefined()
      }
    })
    await updateCustomer(customerId, {
      contact: '张经理',
      name: '示例客户',
      remark: '重点客户',
      status: 'using',
    })
    await enableCustomer(customerId)
  })

  test('loads and deduplicates every customer option page', async () => {
    const requestedPages: string[] = []
    let active = 0
    let maximumActive = 0
    api.defaults.adapter = async (config) => {
      const params = config.params as URLSearchParams
      const page = params.get('p') ?? ''
      requestedPages.push(page)
      active += 1
      maximumActive = Math.max(maximumActive, active)
      await new Promise((resolve) => setTimeout(resolve, 5))
      active -= 1
      const id = page === '3' ? '1' : page
      return {
        config,
        data: {
          code: '',
          data: {
            items: [{ id }],
            p: Number(page),
            page_size: 100,
            total: 501,
          },
          message: '',
          request_id: 'req_customer_options',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }

    const result = await listAllCustomers({
      sort_by: 'name',
      sort_order: 'asc',
    })
    expect(requestedPages.sort()).toEqual(['1', '2', '3', '4', '5', '6'])
    expect(maximumActive).toBe(4)
    expect(result.items.map((item) => String(item.id))).toEqual([
      '1',
      '2',
      '4',
      '5',
      '6',
    ])
  })
})
