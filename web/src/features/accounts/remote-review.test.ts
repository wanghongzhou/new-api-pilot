import { describe, expect, test } from 'bun:test'

import { parseIdString, parseMetricString } from '@/lib/api-types'

import { findExactRemoteUser, remoteUserChanged } from './remote-review'
import type { RemoteUserItem, RemoteUserPage } from './types'

function remoteUser(overrides: Partial<RemoteUserItem> = {}): RemoteUserItem {
  return {
    already_managed: false,
    created_at: 1_700_000_000,
    display_name: '生产账户',
    group: 'vip',
    id: parseIdString('9007199254740993'),
    last_login_at: null,
    managed_account_id: null,
    managed_customer_name: '',
    quota: parseMetricString('1000000'),
    request_count: parseMetricString('50'),
    role: 1,
    status: 1,
    used_quota: parseMetricString('2000000'),
    username: 'customer_prod',
    ...overrides,
  }
}

function page(items: RemoteUserItem[]): RemoteUserPage {
  return { items, page: 1, page_size: 100, total: items.length }
}

describe('remote user precise review', () => {
  test('accepts exactly one matching bigint-safe remote ID', () => {
    const exact = remoteUser()
    expect(
      findExactRemoteUser(
        page([remoteUser({ id: parseIdString('2') }), exact]),
        '9007199254740993'
      )
    ).toBe(exact)
  })

  test('rejects missing and duplicate exact matches', () => {
    const exact = remoteUser()
    expect(findExactRemoteUser(page([]), exact.id)).toBeNull()
    expect(
      findExactRemoteUser(page([exact, { ...exact }]), exact.id)
    ).toBeNull()
  })

  test('detects identity, status, quota and created-at drift', () => {
    const selected = remoteUser()
    expect(remoteUserChanged(selected, { ...selected })).toBeFalse()
    expect(
      remoteUserChanged(selected, { ...selected, username: 'renamed' })
    ).toBeTrue()
    expect(remoteUserChanged(selected, { ...selected, status: 2 })).toBeTrue()
    expect(
      remoteUserChanged(selected, {
        ...selected,
        quota: parseMetricString('999999'),
      })
    ).toBeTrue()
    expect(
      remoteUserChanged(selected, { ...selected, created_at: 1 })
    ).toBeTrue()
  })
})
