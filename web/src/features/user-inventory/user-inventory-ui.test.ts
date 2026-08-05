import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const pagePath = new URL(
  './components/user-inventory-page.tsx',
  import.meta.url
)

describe('user inventory operational fields', () => {
  test('keeps lifecycle evidence visible in desktop and mobile layouts', async () => {
    const source = await readFile(pagePath, 'utf8')

    expect(source).toContain('row.original.remote_created_at')
    expect(source).toContain('row.original.first_seen_at')
    expect(source).toContain('row.original.missing_count')
    expect(source).toContain('item.remote_created_at')
    expect(source).toContain('item.first_seen_at')
    expect(source).toContain('item.missing_count')
    expect(source).toContain("t('common.none')")
  })

  test('adds explicit long-text boundaries to inventory identities', async () => {
    const source = await readFile(pagePath, 'utf8')

    expect(source).toContain('max-w-72')
    expect(source).toContain('break-words')
    expect(source).toContain('break-all')
  })
})
