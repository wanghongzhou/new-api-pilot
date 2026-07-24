import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const platformUsersPage = new URL(
  '../../features/platform-users/components/platform-users-page.tsx',
  import.meta.url
)
const sitesPage = new URL(
  '../../features/sites/components/sites-page.tsx',
  import.meta.url
)
const dataTable = new URL('./data-table.tsx', import.meta.url)
const table = new URL('./table.tsx', import.meta.url)

describe('fixed-height data table layout', () => {
  test.each([
    ['platform users', platformUsersPage],
    ['sites', sitesPage],
  ])(
    '%s keeps DataTable inside a flex column that fills remaining height',
    async (_, path) => {
      const source = await readFile(path, 'utf8')
      expect(source).toMatch(
        /<div className='flex min-h-0 flex-1 flex-col'>\s*<DataTable/
      )
    }
  )

  test('keeps desktop rows inside the table scroller with a sticky header', async () => {
    const [dataTableSource, tableSource] = await Promise.all([
      readFile(dataTable, 'utf8'),
      readFile(table, 'utf8'),
    ])

    expect(dataTableSource).toContain(
      'overflow-auto overscroll-contain focus-visible:ring-2'
    )
    expect(dataTableSource).toContain("containerClassName='overflow-visible'")
    expect(dataTableSource).toContain('containerTabIndex={-1}')
    expect(dataTableSource).toContain(
      "<TableHeader className='sticky top-0 z-10 bg-[var(--table-header)] text-left'>"
    )
    expect(tableSource).toContain('containerClassName?: string')
    expect(tableSource).toContain('containerTabIndex?: number')
  })
})
