import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

describe('shared mobile touch target contract', () => {
  test('keeps interactive primitives at least 40px before desktop breakpoints', async () => {
    const [button, input, tabs, header, brand, appHeader] = await Promise.all([
      readFile(new URL('./button.tsx', import.meta.url), 'utf8'),
      readFile(new URL('./input.tsx', import.meta.url), 'utf8'),
      readFile(new URL('./tabs.tsx', import.meta.url), 'utf8'),
      readFile(new URL('../layout/header.tsx', import.meta.url), 'utf8'),
      readFile(new URL('../layout/brand.tsx', import.meta.url), 'utf8'),
      readFile(new URL('../layout/app-header.tsx', import.meta.url), 'utf8'),
    ])

    expect(button).toContain("icon: 'size-10 sm:size-8'")
    expect(button).toContain("'icon-sm':")
    expect(button).toContain("'size-10 rounded-md sm:size-7")
    expect(input).toContain('h-10 w-full')
    expect(input).toContain('sm:h-8')
    expect(tabs).toContain('group-data-horizontal/tabs:h-10')
    expect(tabs).toContain('relative inline-flex h-10')
    expect(header).toContain("className='size-10 sm:size-8'")
    expect(brand).toContain('inline-flex min-h-10')
    expect(appHeader).toContain('relative size-10 p-0 sm:size-6')
  })

  test('keeps filtering and table controls at least 40px on mobile', async () => {
    const [facetedFilter, viewToggle, columnHeader, select] = await Promise.all(
      [
        readFile(
          new URL('../data/faceted-filter.tsx', import.meta.url),
          'utf8'
        ),
        readFile(
          new URL('../data/data-view-mode-toggle.tsx', import.meta.url),
          'utf8'
        ),
        readFile(
          new URL('./data-table-column-header.tsx', import.meta.url),
          'utf8'
        ),
        readFile(new URL('./select.tsx', import.meta.url), 'utf8'),
      ]
    )

    expect(facetedFilter).toContain("className='h-10 border-dashed sm:h-8'")
    expect(viewToggle).toContain('inline-flex h-12 items-center')
    expect(viewToggle).toContain('inline-flex h-10 w-10')
    expect(columnHeader).toContain('-ms-3 h-10 sm:h-8')
    expect(select).toContain('data-[size=default]:h-10')
    expect(select).toContain('data-[size=sm]:h-10')
  })
})
