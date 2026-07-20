import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const files = ['types.ts', 'api.ts', 'components/subscription-plans-page.tsx']

describe('subscription plan frontend privacy and N+1 boundary', () => {
  test('contains only catalog routes and no sensitive provider or per-plan request primitive', async () => {
    const source = (
      await Promise.all(
        files.map((file) => readFile(new URL(file, import.meta.url), 'utf8'))
      )
    )
      .join('\n')
      .toLowerCase()

    for (const field of [
      ['stripe', 'price', 'id'].join('_'),
      ['creem', 'product', 'id'].join('_'),
      ['waffo', 'product', 'id'].join('_'),
      ['allow', 'balance', 'pay'].join('_'),
      ['wallet', 'overflow'].join('_'),
      ['max', 'purchase'].join('_'),
      ['upgrade', 'group'].join('_'),
      ['downgrade', 'group'].join('_'),
      ['subscription', 'order'].join('_'),
      ['user', 'subscription'].join('_'),
    ]) {
      expect(source).not.toContain(field)
    }
    expect(source).not.toMatch(
      /subscription-plans\/\$\{|subscription-plans\/\$\{/
    )
  })
})
