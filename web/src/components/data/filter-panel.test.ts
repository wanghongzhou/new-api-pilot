import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const componentPath = new URL('./filter-panel.tsx', import.meta.url)

describe('FilterPanel responsive accessibility contract', () => {
  test('keeps cloned controls and action buttons touch-safe on mobile', async () => {
    const source = await readFile(componentPath, 'utf8')
    expect(source).toContain("'min-h-10 sm:min-h-8'")
    expect(source.match(/\[&_\[data-slot=button\]\]:min-h-10/g)).toHaveLength(3)
    expect(source.match(/sm:\[&_\[data-slot=button\]\]:min-h-8/g)).toHaveLength(
      3
    )
  })
})
