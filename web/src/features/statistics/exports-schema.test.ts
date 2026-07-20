import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { exportListParams, hasExportFilters } from './exports-contract'
import { exportsSearchSchema } from './exports-schema'
import type { StatisticsExportSearch } from './types'

describe('export task URL and list contract', () => {
  test('keeps bigint export IDs as strings and accepts documented filters', () => {
    expect(
      exportsSearchSchema.parse({
        exportId: '9007199254740993',
        format: 'xlsx',
        order: 'asc',
        page: '2',
        pageSize: '100',
        scope: 'account',
        sort: 'file_size',
        status: 'success',
      })
    ).toEqual({
      exportId: parseIdString('9007199254740993'),
      format: 'xlsx',
      order: 'asc',
      page: 2,
      pageSize: 100,
      scope: 'account',
      sort: 'file_size',
      status: ['success'],
    })
  })

  test('drops invalid deep-link values without number-coercing IDs', () => {
    expect(
      exportsSearchSchema.parse({
        exportId: 9_007_199_254_740_992,
        format: 'pdf',
        page: '0',
        pageSize: '101',
        scope: 'user',
        sort: 'updated_at',
        status: 'cancelled',
      })
    ).toEqual({
      exportId: undefined,
      format: undefined,
      page: undefined,
      pageSize: undefined,
      scope: undefined,
      sort: undefined,
      status: [],
    })
  })

  test('accepts a legacy single status and canonicalizes repeated statuses', () => {
    expect(exportsSearchSchema.parse({ status: 'success' }).status).toEqual([
      'success',
    ])
    expect(
      exportsSearchSchema.parse({
        status: ['failed', 'pending', 'failed'],
      }).status
    ).toEqual(['pending', 'failed'])
  })

  test('maps URL state to the exact current-user list query', () => {
    const search: StatisticsExportSearch = {
      format: 'csv',
      order: 'desc',
      page: 3,
      pageSize: 20,
      scope: 'channel',
      sort: 'finished_at',
      status: ['failed', 'expired'],
    }
    expect(exportListParams(search)).toEqual({
      format: 'csv',
      p: 3,
      page_size: 20,
      sort_by: 'finished_at',
      sort_order: 'desc',
      statistics_type: 'channel',
      status: ['failed', 'expired'],
    })
    expect(hasExportFilters(search)).toBeTrue()
    expect(
      hasExportFilters({
        order: 'desc',
        page: 1,
        pageSize: 20,
        sort: 'created_at',
        status: [],
      })
    ).toBeFalse()
  })
})
