import { describe, expect, test } from 'bun:test'

import { isConsumptionLogType } from './display'
import type { LogType } from './types'

describe('log display semantics', () => {
  test('only consumption logs expose model usage and billing metrics', () => {
    const types: LogType[] = [0, 1, 2, 3, 4, 5, 6, 7]

    expect(types.filter(isConsumptionLogType)).toEqual([2])
  })
})
