import { expect, test } from 'bun:test'

import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'

import { buildLogExportRequest } from './export-request'

test('log export freezes all filters and omits pagination', () => {
  const request = buildLogExportRequest('xlsx', {
    channelId: parseNonNegativeIdString('9007199254740997'),
    end: 1_784_262_400,
    group: 'vip',
    modelName: 'gpt-4.1',
    page: 9,
    pageSize: 100,
    requestId: 'req-local',
    siteIds: [parseIdString('9007199254740993')],
    start: 1_784_176_000,
    tokenName: 'production',
    type: 2,
    upstreamRequestId: 'req-upstream',
    username: 'alice',
  })
  expect(request).toEqual({
    filters: {
      account_ids: [],
      channel_id: '9007199254740997',
      channel_keys: [],
      customer_ids: [],
      end_timestamp: 1_784_262_400,
      granularity: 'hour',
      log_type: 2,
      model_names: ['gpt-4.1'],
      node_names: [],
      request_id: 'req-local',
      site_ids: [parseIdString('9007199254740993')],
      sort_by: 'name',
      sort_order: 'desc',
      start_timestamp: 1_784_176_000,
      token_keys: [],
      token_name: 'production',
      upstream_request_id: 'req-upstream',
      use_groups: ['vip'],
      username: 'alice',
    },
    format: 'xlsx',
    statistics_type: 'logs',
  })
  expect(request.filters).not.toHaveProperty('p')
  expect(request.filters).not.toHaveProperty('page_size')
})
