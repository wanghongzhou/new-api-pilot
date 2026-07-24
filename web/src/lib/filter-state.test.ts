import { describe, expect, test } from 'bun:test'

import { hasFilterChanges } from './filter-state'

describe('hasFilterChanges', () => {
  const baseline = { keyword: '', page: 1, siteIds: [] as string[] }

  test('ignores pagination fields outside the selected filter keys', () => {
    expect(
      hasFilterChanges({ ...baseline, page: 2 }, baseline, [
        'keyword',
        'siteIds',
      ])
    ).toBe(false)
  })

  test('detects scalar and list filter changes', () => {
    expect(
      hasFilterChanges({ ...baseline, keyword: 'alice' }, baseline, [
        'keyword',
        'siteIds',
      ])
    ).toBe(true)
    expect(
      hasFilterChanges({ ...baseline, siteIds: ['1'] }, baseline, [
        'keyword',
        'siteIds',
      ])
    ).toBe(true)
  })
})
