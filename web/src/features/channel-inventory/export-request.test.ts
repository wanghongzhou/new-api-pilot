import { expect, test } from 'bun:test'

import {
  parseDecimalString,
  parseIdString,
  parseMetricString,
} from '@/lib/api-types'

import { buildChannelInventoryExportRequest } from './export-request'

test('channel inventory export freezes safe filters without pagination or key material', () => {
  const request = buildChannelInventoryExportRequest('xlsx', {
    end: 1_784_348_800,
    groups: ['vip'],
    keyword: 'gpt',
    maxBalance: parseDecimalString('9007199254740993.1234567890'),
    maxResponseTime: parseMetricString('9223372036854775807'),
    page: 9,
    pageSize: 100,
    siteIds: [parseIdString('9007199254740993')],
    start: 1_784_262_400,
    states: ['missing'],
    statuses: [3],
    tags: ['primary'],
    types: [8],
  })
  expect(request.statistics_type).toBe('channel_inventory')
  expect(request.filters).toMatchObject({
    channel_states: ['missing'],
    channel_statuses: [3],
    channel_tags: ['primary'],
    channel_types: [8],
    max_balance: '9007199254740993.1234567890',
    max_response_time_ms: '9223372036854775807',
    site_ids: [parseIdString('9007199254740993')],
    use_groups: ['vip'],
  })
  expect(request.filters).not.toHaveProperty('p')
  expect(request.filters).not.toHaveProperty('page_size')
  expect(request.filters).not.toHaveProperty('key')
  expect(request.filters).not.toHaveProperty('multi_key')
  expect(request.filters).not.toHaveProperty('base_url')
  expect(request.filters).not.toHaveProperty('headers')
})
