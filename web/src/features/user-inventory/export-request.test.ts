import { expect, test } from 'bun:test'

import { parseIdString, parseMetricString } from '@/lib/api-types'

import { buildUserInventoryExportRequest } from './export-request'

test('user inventory export freezes only safe list filters without pagination', () => {
  const request = buildUserInventoryExportRequest('csv', {
    end: 1_784_262_400,
    groups: ['vip'],
    keyword: 'alice',
    maxBalance: parseMetricString('9223372036854775807'),
    minBalance: parseMetricString('-10'),
    page: 9,
    pageSize: 100,
    roles: [1, 100],
    remoteUserId: parseIdString('9007199254740997'),
    siteIds: [parseIdString('9007199254740993')],
    start: 1_784_176_000,
    states: ['missing'],
    statuses: [1],
  })
  expect(request.statistics_type).toBe('user_inventory')
  expect(request.filters).toMatchObject({
    inventory_roles: [1, 100],
    inventory_states: ['missing'],
    inventory_statuses: [1],
    keyword: 'alice',
    max_balance: '9223372036854775807',
    min_balance: '-10',
    remote_user_id: parseIdString('9007199254740997'),
    site_ids: [parseIdString('9007199254740993')],
    use_groups: ['vip'],
  })
  expect(request.filters).not.toHaveProperty('p')
  expect(request.filters).not.toHaveProperty('page_size')
})
