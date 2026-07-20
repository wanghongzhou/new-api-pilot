import { describe, expect, test } from 'bun:test'

import { parseRouterSearch, stringifyRouterSearch } from './router-search'

describe('router search serialization', () => {
  test('keeps bigint ID strings readable and exact', () => {
    const runId = '9223372036854775807'

    expect(stringifyRouterSearch({ runId })).toBe(`?runId=${runId}`)
    expect(parseRouterSearch(`?runId=${runId}`)).toEqual({ runId })
  })

  test('keeps safe-integer ID search values as strings', () => {
    expect(parseRouterSearch('?alertId=999&ruleSiteId=801&page=2')).toEqual({
      alertId: '999',
      page: 2,
      ruleSiteId: '801',
    })
  })

  test('continues to parse previously quoted ID deep links', () => {
    expect(parseRouterSearch('?runId=%229223372036854775807%22')).toEqual({
      runId: '9223372036854775807',
    })
  })

  test('retains JSON serialization for structured search values', () => {
    const search = { page: 2, statuses: ['pending', 'running'] }
    const serialized = stringifyRouterSearch(search)

    expect(parseRouterSearch(serialized)).toEqual({
      page: 2,
      statuses: ['pending', 'running'],
    })
  })
})
