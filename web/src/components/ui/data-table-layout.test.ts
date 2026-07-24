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
})
