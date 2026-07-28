import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { buildSystemTaskSearch, changeSystemTaskTab } from './search'

describe('system task URL search', () => {
  test('normalizes bigint sites, six types, four statuses, tab and time range', () => {
    const search = buildSystemTaskSearch({
      createdEnd: 200,
      createdStart: 100,
      errorPresent: false,
      page: 2,
      pageSize: 50,
      siteIds: ['9007199254740995', '9007199254740993'],
      statuses: ['failed', 'pending', 'failed'],
      tab: 'types',
      types: ['model_update', 'log_cleanup', 'log_detail_cleanup'],
    })
    expect(search.siteIds).toEqual([
      parseIdString('9007199254740993'),
      parseIdString('9007199254740995'),
    ])
    expect(search.statuses).toEqual(['pending', 'failed'])
    expect(search.types).toEqual([
      'log_cleanup',
      'log_detail_cleanup',
      'model_update',
    ])
    expect(search.tab).toBe('types')
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
      tab: 'list',
      types: [],
    })
  })
  test('clears list-only filters when entering analysis', () => {
    expect(changeSystemTaskTab('types')).toEqual({
      createdEnd: undefined,
      createdStart: undefined,
      errorPresent: undefined,
      page: 1,
      pageSize: 20,
      siteIds: [],
      statuses: [],
      tab: 'types',
      types: [],
    })
    expect(changeSystemTaskTab('list')).toEqual({ page: 1, tab: 'list' })
  })
})
