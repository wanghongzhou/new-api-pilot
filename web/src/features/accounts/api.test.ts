import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'

import {
  archiveAccount,
  createAccount,
  deleteAccount,
  getAccount,
  getAccountStatistics,
  listAccounts,
  listRemoteUsers,
  refreshAccount,
  restoreAccount,
  updateAccount,
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
        request_id: 'req_account',
        success: true,
      },
      headers: {},
      status: 200,
      statusText: 'OK',
    }
  }
}

describe('account API', () => {
  test('serializes account filters as repeated query keys', async () => {
    api.defaults.adapter = successAdapter((config) => {
      const params = config.params as URLSearchParams
      expect(config.url).toBe('/api/accounts')
      expect(params.getAll('remote_state')).toEqual([
        'missing',
        'identity_mismatch',
      ])
      expect(params.getAll('managed_status')).toEqual(['active', 'archived'])
      expect(params.getAll('remote_status')).toEqual(['1', '2'])
      expect(params.get('customer_id')).toBe('9007199254740993')
    })
    await listAccounts({
      customer_id: parseIdString('9007199254740993'),
      managed_status: ['active', 'archived'],
      remote_state: ['missing', 'identity_mismatch'],
      remote_status: ['1', '2'],
    })
  })

  test('posts only immutable binding IDs and the optional remark', async () => {
    api.defaults.adapter = successAdapter((config) => {
      expect(config.method).toBe('post')
      expect(config.url).toBe('/api/accounts')
      expect(JSON.parse(String(config.data))).toEqual({
        customer_id: '9007199254740993',
        remote_user_id: '9007199254740995',
        remark: '生产账户',
        site_id: '9007199254740994',
      })
    })
    await createAccount({
      customer_id: parseIdString('9007199254740993'),
      remote_user_id: parseIdString('9007199254740995'),
      remark: '生产账户',
      site_id: parseIdString('9007199254740994'),
    })
  })

  test('uses the account stats endpoint and sends restore with no body', async () => {
    const accountId = parseIdString('88')
    let call = 0
    api.defaults.adapter = successAdapter((config) => {
      call += 1
      if (call === 1) {
        const params = config.params as URLSearchParams
        expect(config.url).toBe('/api/accounts/88/stats')
        expect(params.get('granularity')).toBe('day')
        expect(params.get('start_timestamp')).toBe('1781280000')
        expect(params.get('end_timestamp')).toBe('1783872000')
      } else {
        expect(config.url).toBe('/api/accounts/88/restore')
        expect(config.data).toBeUndefined()
      }
    })
    await getAccountStatistics(accountId, {
      end_timestamp: 1_783_872_000,
      granularity: 'day',
      p: 1,
      page_size: 20,
      sort_by: 'quota',
      sort_order: 'desc',
      start_timestamp: 1_781_280_000,
    })
    await restoreAccount(accountId)
  })

  test('uses bigint-safe detail and mutation routes without leaking binding fields', async () => {
    const accountId = parseIdString('9007199254740993')
    const expected = [
      ['get', '/api/accounts/9007199254740993', undefined],
      ['put', '/api/accounts/9007199254740993', { remark: 'production owner' }],
      ['post', '/api/accounts/9007199254740993/archive', undefined],
      ['post', '/api/accounts/9007199254740993/refresh', undefined],
      ['delete', '/api/accounts/9007199254740993', undefined],
    ] as const
    let call = 0
    api.defaults.adapter = successAdapter((config) => {
      const request = expected[call]
      if (!request) throw new Error(`unexpected account API call ${call + 1}`)
      const [method, url, body] = request
      expect(config.method).toBe(method)
      expect(config.url).toBe(url)
      expect(
        config.data == null ? undefined : JSON.parse(String(config.data))
      ).toEqual(body)
      call += 1
    })

    await getAccount(accountId)
    await updateAccount(accountId, { remark: 'production owner' })
    await archiveAccount(accountId)
    await refreshAccount(accountId)
    await deleteAccount(accountId)
    expect(call).toBe(expected.length)
  })

  test('searches remote users on the selected site with explicit pagination', async () => {
    api.defaults.adapter = successAdapter((config) => {
      const params = config.params as URLSearchParams
      expect(config.method).toBe('get')
      expect(config.url).toBe(
        '/api/accounts/site/9007199254740994/remote-users'
      )
      expect(params.get('keyword')).toBe('alice')
      expect(params.get('p')).toBe('2')
      expect(params.get('page_size')).toBe('100')
    })
    await listRemoteUsers(parseIdString('9007199254740994'), {
      keyword: 'alice',
      p: 2,
      page_size: 100,
    })
  })
})
