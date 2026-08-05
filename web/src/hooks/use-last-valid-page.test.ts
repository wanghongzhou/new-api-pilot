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
