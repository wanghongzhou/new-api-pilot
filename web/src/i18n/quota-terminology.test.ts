import { describe, expect, test } from 'bun:test'

import zhCN from './locales/zh-CN.json'

describe('quota terminology', () => {
  test('uses 额度 in user-visible Chinese copy', () => {
    const untranslatedKeys = Object.entries(zhCN)
      .filter(([, value]) =>
        value
          .replaceAll(/{{[^}]+}}/g, '')
          .toLowerCase()
          .includes('quota')
      )
      .map(([key]) => key)

    expect(untranslatedKeys).toEqual([])
  })
})
