import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const entityWorkspace = new URL(
  './components/entity-statistics.tsx',
  import.meta.url
)
const globalWorkspace = new URL(
  './components/statistics-page.tsx',
  import.meta.url
)

describe('statistics summary information hierarchy', () => {
  test('does not repeat raw quota as a fifth summary metric', async () => {
    const source = await readFile(entityWorkspace, 'utf8')

    expect(source).toContain("search.display !== 'quota' && (")
    expect(source).toContain('lg:grid-cols-4')
  })

  test.each([entityWorkspace, globalWorkspace])(
    'does not repeat raw quota as an amount table column in %s',
    async (workspace) => {
      const source = await readFile(workspace, 'utf8')

      expect(source).toContain("...(search.display === 'quota'")
      expect(source).toContain("id: 'amount'")
    }
  )
})
