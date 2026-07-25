import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const auditedFiles = [
  '../accounts/components/account-detail-page.tsx',
  '../accounts/components/account-onboarding-drawer.tsx',
  '../accounts/components/account-ui.tsx',
  '../customers/components/customer-detail-page.tsx',
  '../customers/components/customer-ui.tsx',
  './components/entity-statistics.tsx',
  './components/export-dialog.tsx',
  './components/export-task-sheet.tsx',
  './components/exports-page.tsx',
  './components/statistics-page.tsx',
] as const

describe('definition list accessibility boundary', () => {
  test('keeps dt and dd grouped in independent definition lists', async () => {
    for (const file of auditedFiles) {
      const source = await readFile(new URL(file, import.meta.url), 'utf8')
      expect(source, file).not.toMatch(/<dl\b[^>]*>\s*<div\b/s)
    }
  })
})
