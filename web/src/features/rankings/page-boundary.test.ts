import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

describe('rankings page failure and forced-site boundaries', () => {
  test('keeps the table error state visible without response data', async () => {
    const source = await readFile(
      new URL('components/rankings-page.tsx', import.meta.url),
      'utf8'
    )

    expect(source).toContain(
      'data={pageItems(data?.items ?? [], search.page, search.pageSize)}'
    )
    expect(source).toContain('total={data?.items.length ?? 0}')
    expect(source).toContain('error={!validSite || rankingQuery.isError}')
    expect(source).toContain(
      'onRetry={validSite ? () => void rankingQuery.refetch() : undefined}'
    )
  })

  test('blocks export when the forced site path is invalid', async () => {
    const source = await readFile(
      new URL('components/rankings-page.tsx', import.meta.url),
      'utf8'
    )

    expect(source).toContain(
      'disabled={exportMutation.isPending || !validSite}'
    )
  })

  test('formats authoritative bigint metrics without number conversion', async () => {
    const source = await readFile(
      new URL('components/rankings-page.tsx', import.meta.url),
      'utf8'
    )

    expect(source).toContain(
      "import { MetricValue } from '@/components/data/metric-value'"
    )
    expect(source).toContain('<MetricValue value={value} />')
    expect(source).not.toContain('Number(item.token_used)')
    expect(source).not.toContain('Number(row.original.token_used)')
  })
})
