import { describe, expect, test } from 'bun:test'

import { lastValidPage, pageReplacement } from './use-last-valid-page'

describe('lastValidPage', () => {
  test('calculates bounded pages without exposing page zero', () => {
    expect(lastValidPage(0, 20)).toBe(1)
    expect(lastValidPage(1, 20)).toBe(1)
    expect(lastValidPage(21, 20)).toBe(2)
    expect(lastValidPage(21, 0)).toBe(1)
  })

  test('replaces page 999 for populated and empty results', () => {
    expect(
      pageReplacement({
        isFetching: false,
        isPlaceholderData: false,
        page: 999,
        pageSize: 20,
        total: 21,
      })
    ).toBe(2)
    expect(
      pageReplacement({
        isFetching: false,
        isPlaceholderData: false,
        page: 999,
        pageSize: 20,
        total: 0,
      })
    ).toBe(1)
  })

  test('uses bigint-safe totals and jumps directly when the last page is safe', () => {
    expect(
      pageReplacement({
        isFetching: false,
        isPlaceholderData: false,
        page: 999,
        pageSize: 20,
        total: '9007199254740993',
      })
    ).toBeUndefined()
    expect(
      pageReplacement({
        isFetching: false,
        isPlaceholderData: false,
        page: 3,
        pageSize: 20,
        total: '21',
      })
    ).toBe(2)
    expect(
      pageReplacement({
        isFetching: false,
        isPlaceholderData: false,
        page: 999_999,
        pageSize: 20,
        total: '21',
      })
    ).toBe(2)
  })

  test('retreats sequentially only when the exact last page is unsafe', () => {
    expect(
      pageReplacement({
        isFetching: false,
        isPlaceholderData: false,
        page: 999,
        pageSize: 1,
        total: '90071992547409930',
      })
    ).toBeUndefined()
  })

  test('does not replace while a placeholder or request is active', () => {
    expect(
      pageReplacement({
        isFetching: true,
        isPlaceholderData: false,
        page: 999,
        pageSize: 20,
        total: 21,
      })
    ).toBeUndefined()
    expect(
      pageReplacement({
        isFetching: false,
        isPlaceholderData: true,
        page: 999,
        pageSize: 20,
        total: 21,
      })
    ).toBeUndefined()
  })
})
