import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { buildSystemTaskExportRequest } from './export-request'
import { buildSystemTaskSearch } from './search'

describe('system task export contract', () => {
  test('freezes only safe list filters', () => {
    const request = buildSystemTaskExportRequest(
      'xlsx',
      buildSystemTaskSearch({
        createdEnd: 200,
        createdStart: 100,
        errorPresent: true,
        siteIds: ['9007199254740993'],
        statuses: ['failed'],
        types: ['channel_test'],
      })
    )
    expect(request).toMatchObject({
      format: 'xlsx',
      statistics_type: 'system_tasks',
      filters: {
        created_end: 200,
        created_start: 100,
        error_present: true,
        site_ids: [parseIdString('9007199254740993')],
        statuses: ['failed'],
        types: ['channel_test'],
      },
    })
  })
  test('forced site replaces global site ids', () => {
    expect(
      buildSystemTaskExportRequest(
        'csv',
        buildSystemTaskSearch({ siteIds: ['9'] }),
        parseIdString('9007199254740993')
      ).filters.site_ids
    ).toEqual([parseIdString('9007199254740993')])
  })
})
