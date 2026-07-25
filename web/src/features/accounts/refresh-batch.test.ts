import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { refreshAccountsBounded } from './refresh-batch'

describe('bounded account refresh', () => {
  test('limits concurrent upstream refreshes and counts failures', async () => {
    const ids = Array.from({ length: 12 }, (_, index) =>
      parseIdString(String(index + 1))
    )
    let active = 0
    let peak = 0
    const result = await refreshAccountsBounded(
      ids,
      async (id) => {
        active += 1
        peak = Math.max(peak, active)
        await Promise.resolve()
        active -= 1
        if (id === '3' || id === '9') throw new Error('refresh failed')
      },
      3
    )

    expect(peak).toBe(3)
    expect(result).toEqual({ failed: 2, total: 12 })
  })

  test('handles an empty page without starting workers', async () => {
    let called = false
    expect(
      await refreshAccountsBounded([], async () => {
        called = true
      })
    ).toEqual({ failed: 0, total: 0 })
    expect(called).toBe(false)
  })

  test('falls back to the safe default for a non-finite concurrency', async () => {
    const ids = Array.from({ length: 6 }, (_, index) =>
      parseIdString(String(index + 1))
    )
    let calls = 0

    const result = await refreshAccountsBounded(
      ids,
      async () => {
        calls += 1
      },
      Number.NaN
    )

    expect(calls).toBe(6)
    expect(result).toEqual({ failed: 0, total: 6 })
  })
})
