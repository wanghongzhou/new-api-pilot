import { expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const cardDestinationPages = [
  './components/site-stats-page.tsx',
  './components/site-instances-page.tsx',
] as const

test('returns site card destination pages directly to site management', async () => {
  for (const file of cardDestinationPages) {
    const source = await readFile(new URL(file, import.meta.url), 'utf8')

    expect(source, file).toContain("to='/sites'")
    expect(source, file).toContain('statistics: []')
    expect(source, file).toContain("t('site.backToManagement')")
    expect(source, file).not.toContain("to='/sites/$siteId'")
  }
})
