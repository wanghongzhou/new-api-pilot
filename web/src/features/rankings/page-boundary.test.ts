import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

describe('rankings page failure and forced-site boundaries', () => {
  test('keeps the table error state visible without response data', async () => {
    const source = await readFile(
      new URL('components/rankings-page.tsx', import.meta.url),
      'utf8'
    )

    expect(source).toContain('data={data?.items ?? []}')
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
})
