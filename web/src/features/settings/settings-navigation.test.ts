import { describe, expect, mock, test } from 'bun:test'

import { revealHorizontalTarget } from './settings-navigation'

const rect = (left: number, right: number) => ({ left, right })

describe('settings category navigation', () => {
  test('reveals a deep-linked category hidden after the right edge', () => {
    const scrollBy = mock(() => undefined)
    revealHorizontalTarget(
      { getBoundingClientRect: () => rect(10, 390), scrollBy },
      { getBoundingClientRect: () => rect(410, 490) }
    )
    expect(scrollBy).toHaveBeenCalledWith({ behavior: 'auto', left: 100 })
  })

  test('reveals a category hidden before the left edge', () => {
    const scrollBy = mock(() => undefined)
    revealHorizontalTarget(
      { getBoundingClientRect: () => rect(20, 400), scrollBy },
      { getBoundingClientRect: () => rect(-30, 70) }
    )
    expect(scrollBy).toHaveBeenCalledWith({ behavior: 'auto', left: -50 })
  })

  test('does not move an already fully visible category', () => {
    const scrollBy = mock(() => undefined)
    revealHorizontalTarget(
      { getBoundingClientRect: () => rect(0, 390), scrollBy },
      { getBoundingClientRect: () => rect(90, 180) }
    )
    expect(scrollBy).not.toHaveBeenCalled()
  })
})
