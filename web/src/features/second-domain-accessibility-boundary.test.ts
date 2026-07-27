import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const auditedFiles = [
  './model-catalog/components/model-catalog-page.tsx',
  './platform-users/components/platform-users-page.tsx',
  './pricing-groups/components/pricing-groups-page.tsx',
  './sites/components/collection-runs-panel.tsx',
  './sites/components/site-dialogs.tsx',
  './sites/components/site-instances-page.tsx',
  './sites/components/site-onboarding-drawer.tsx',
] as const

describe('second-domain accessibility boundary', () => {
  test('keeps definition terms and descriptions in valid groups', async () => {
    for (const file of auditedFiles) {
      const source = await readFile(new URL(file, import.meta.url), 'utf8')
      expect(source, file).not.toMatch(/<dl\b[^>]*>\s*<div\b/s)
    }
  })
})
