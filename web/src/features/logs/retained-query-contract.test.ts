import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

describe('logs retained query contract', () => {
  test('retains list and statistics without crossing global and site scopes', async () => {
    const source = await readFile(
      new URL('./components/logs-page.tsx', import.meta.url),
      'utf8'
    )

    expect(source).toContain('useRetainedQueryData')
    expect(source).toContain("siteId ? `site:${siteId}` : 'global'")
    expect(source).toContain('common.retainedDataRefreshFailed')
    expect(source).toContain('logs.stats.refreshFailed')
  })
})
