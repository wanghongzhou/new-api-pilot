import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const pagePath = new URL('./components/model-catalog-page.tsx', import.meta.url)

describe('model catalog icon boundary', () => {
  test('renders icon only as text and contains no external resource primitive', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain('{row.original.icon ||')
    expect(source).toContain('{item.icon ||')
    expect(source).not.toContain('<img')
    expect(source).not.toContain('<source')
    expect(source).not.toContain('backgroundImage')
    expect(source).not.toContain('href={item.icon')
    expect(source).not.toContain('src={item.icon')
  })
})
