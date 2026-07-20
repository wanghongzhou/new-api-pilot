import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { buildSystemTaskSearch } from './search'

describe('system task URL search', () => {
  test('normalizes bigint sites, five types, four statuses and time range', () => {
    const search = buildSystemTaskSearch({
      createdEnd: 200,
      createdStart: 100,
      errorPresent: false,
      page: 2,
      pageSize: 50,
      siteIds: ['9007199254740995', '9007199254740993'],
      statuses: ['failed', 'pending', 'failed'],
      types: ['model_update', 'log_cleanup'],
    })
    expect(search.siteIds).toEqual([
      parseIdString('9007199254740993'),
      parseIdString('9007199254740995'),
    ])
    expect(search.statuses).toEqual(['pending', 'failed'])
    expect(search.types).toEqual(['log_cleanup', 'model_update'])
    expect(search.errorPresent).toBe(false)
  })
  test('fails closed for invalid values and reversed time', () => {
    expect(
      buildSystemTaskSearch({
        createdEnd: 100,
        createdStart: 200,
        page: 0,
        pageSize: 101,
        siteIds: ['0'],
        statuses: ['UNKNOWN'],
        types: ['delete_all'],
      })
    ).toMatchObject({
      createdEnd: undefined,
      createdStart: undefined,
      page: 1,
      pageSize: 20,
      siteIds: [],
      statuses: [],
      types: [],
    })
  })
})
