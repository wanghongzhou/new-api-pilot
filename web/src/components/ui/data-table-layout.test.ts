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
const pagination = new URL('./data-table-pagination.tsx', import.meta.url)
const dialog = new URL('./dialog.tsx', import.meta.url)
const drawer = new URL('./drawer.tsx', import.meta.url)

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
    expect(dataTableSource).toContain(
      'overflow-y-auto pb-16 transition-opacity duration-150 min-[641px]:hidden'
    )
    expect(dataTableSource).toContain('aria-label={ariaLabel}')
    expect(dataTableSource).toContain("role='region'")
    expect(tableSource).toContain('containerClassName?: string')
    expect(tableSource).toContain('containerTabIndex?: number')
  })

  test('keeps mobile pagination controls at least 40px tall', async () => {
    const source = await readFile(pagination, 'utf8')

    expect(source).toContain('data-[size=default]:h-10')
    expect(source).toContain('size-10 p-0 sm:size-8')
    expect(source).toContain(
      'hidden items-center gap-1 @lg/pagination:flex @xl/pagination:gap-2'
    )
  })

  test('bounds generic overlays to the dynamic viewport and allows scrolling', async () => {
    const [dialogSource, drawerSource] = await Promise.all([
      readFile(dialog, 'utf8'),
      readFile(drawer, 'utf8'),
    ])

    expect(dialogSource).toContain('max-h-[calc(100dvh-2rem)]')
    expect(dialogSource).toContain('overflow-y-auto overscroll-contain')
    expect(drawerSource).toContain('overflow-y-auto overscroll-contain')
    expect(drawerSource).toContain('max-h-[80dvh]')
  })
})
