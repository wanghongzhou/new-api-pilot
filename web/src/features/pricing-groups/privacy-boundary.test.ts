import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const files = ['types.ts', 'api.ts', 'components/pricing-groups-page.tsx']

describe('pricing/group privacy and passive rendering boundary', () => {
  test('omits sensitive enrichment and URL-loading primitives', async () => {
    const source = (
      await Promise.all(
        files.map((file) => readFile(new URL(file, import.meta.url), 'utf8'))
      )
    )
      .join('\n')
      .toLowerCase()
    for (const field of [
      ['billing', 'expr'].join('_'),
      ['custom', 'path'].join('_'),
      ['channel', 'key'].join('_'),
      ['authorization'].join('_'),
      ['header', 'override'].join('_'),
      ['param', 'override'].join('_'),
      ['bound', 'channel'].join('_'),
      ['oauth', 'token'].join('_'),
    ])
      expect(source).not.toContain(field)
    expect(source).not.toMatch(/<img|src=|href=.*endpoint|window\.open/)
  })
})
