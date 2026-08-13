import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

function source(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

describe('CRUD pending closure contract', () => {
  test('keeps account edit open and disables cancel while saving', () => {
    const accountDialogs = source('./accounts/components/account-dialogs.tsx')
    expect(accountDialogs).toContain(
      'onOpenChange={(open) => !open && !pending && requestClose()}'
    )
    expect(accountDialogs).toContain(
      "<Button disabled={pending} onClick={requestClose} variant='outline'>"
    )
  })

  test('keeps alert rule edit open and disables cancel while saving', () => {
    const alertDialogs = source('./alerts/components/alert-rule-dialogs.tsx')
    expect(alertDialogs).toContain(
      '!open && !mutation.isPending && requestClose()'
    )
    expect(alertDialogs).toContain('disabled={mutation.isPending}')
  })

  test('keeps platform-user enable confirmation open while submitting', () => {
    const userDialogs = source('./platform-users/components/user-dialogs.tsx')
    expect(userDialogs).toContain('else if (!submitting) onOpenChange(false)')
    expect(userDialogs).toContain(
      '<DialogCancelButton disabled={submitting} />'
    )
  })
})
