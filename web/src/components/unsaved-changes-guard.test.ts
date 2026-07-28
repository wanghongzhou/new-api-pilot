import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

import { shouldConfirmDiscard } from '@/hooks/use-unsaved-changes-guard'

describe('unsaved changes close guard', () => {
  test('prompts only when the active workflow has changes', () => {
    expect(shouldConfirmDiscard(false)).toBe(false)
    expect(shouldConfirmDiscard(true)).toBe(true)
  })

  test('covers every complex drawer close path with the shared guard', () => {
    const files = [
      '../features/sites/components/site-onboarding-drawer.tsx',
      '../features/sites/components/site-dialogs.tsx',
      '../features/customers/components/customer-dialogs.tsx',
      '../features/accounts/components/account-onboarding-drawer.tsx',
      '../features/accounts/components/account-dialogs.tsx',
      '../features/platform-users/components/user-dialogs.tsx',
      '../features/alerts/components/alert-rule-dialogs.tsx',
    ]

    for (const file of files) {
      const source = readFileSync(new URL(file, import.meta.url), 'utf8')
      expect(source).toContain('useUnsavedChangesGuard')
      expect(source).toContain('UnsavedChangesConfirmDialog')
      expect(source).toContain('requestClose')
    }
  })
})
