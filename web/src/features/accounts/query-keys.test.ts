import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { accountKeys } from './query-keys'

describe('account query keys', () => {
  test('normalizes object order and multi-value filter order', () => {
    const first = accountKeys.list({
      customer_id: parseIdString('9007199254740993'),
      managed_status: ['archived', 'active'],
      p: 1,
    })
    const second = accountKeys.list({
      p: 1,
      managed_status: ['active', 'archived'],
      customer_id: parseIdString('9007199254740993'),
    })

    expect(first).toEqual(second)
  })

  test('keeps detail and statistics caches isolated by bigint-safe ID', () => {
    const params = {
      end_timestamp: 1_783_872_000,
      granularity: 'day',
      start_timestamp: 1_781_280_000,
    }
    expect(accountKeys.detail('9007199254740993')).not.toEqual(
      accountKeys.detail('9007199254740994')
    )
    expect(accountKeys.statistics('9007199254740993', params)).toEqual(
      accountKeys.statistics('9007199254740993', {
        start_timestamp: 1_781_280_000,
        granularity: 'day',
        end_timestamp: 1_783_872_000,
      })
    )
  })

  test('keeps remote-user search pages isolated while normalizing params', () => {
    const first = accountKeys.remoteUsers('9', {
      keyword: 'alice',
      p: 1,
      page_size: 20,
    })
    expect(first).toEqual(
      accountKeys.remoteUsers('9', {
        page_size: 20,
        p: 1,
        keyword: 'alice',
      })
    )
    expect(first).not.toEqual(
      accountKeys.remoteUsers('9', {
        keyword: 'alice',
        p: 2,
        page_size: 20,
      })
    )
  })
})
