import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const pages = [
  [
    'financial operations',
    new URL(
      '../financial-operations/components/financial-operations-page.tsx',
      import.meta.url
    ),
  ],
  [
    'performance history',
    new URL(
      '../performance-history/components/performance-history-page.tsx',
      import.meta.url
    ),
  ],
] as const

describe('operations analytics bigint pagination contract', () => {
  for (const [name, file] of pages) {
    test(`${name} preserves exact totals and uses sequential pagination`, async () => {
      const source = await readFile(file, 'utf8')

      expect(source).toContain('paginationHasKnownLastPage={false}')
      expect(source).toContain('paginationTotalDisplay=')
      expect(source).toContain('BigInt(')
      expect(source).not.toMatch(/total=\{[^}]*\.total/)
    })
  }

  test('financial operations formats bigint and decimal values without losing the originals', async () => {
    const source = await readFile(pages[0][1], 'utf8')

    expect(source).toContain('formatMetricDisplayValue(item.amount)')
    expect(source).toContain('formatDecimalDisplayValue(item.money)')
    expect(source).toContain('title={item.amount}')
    expect(source).toContain('title={item.money}')
    expect(source).toContain('formatMetricDisplayValue(item.quota)')
    expect(source).toContain('title={item.quota}')
    expect(source).toContain(
      'formatDecimalDisplayValue(item.money ?? zeroDecimal)'
    )
  })
})
