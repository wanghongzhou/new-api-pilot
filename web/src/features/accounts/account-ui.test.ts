import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const pagePath = new URL('./components/accounts-page.tsx', import.meta.url)
const cardPath = new URL('./components/account-ui.tsx', import.meta.url)

describe('account management list parity', () => {
  test('matches the site card metric hierarchy', async () => {
    const source = await readFile(cardPath, 'utf8')

    expect(source).toContain('text-base leading-tight font-semibold')
    expect(source).toContain('account.today.token_used')
    expect(source).toContain('account.today.avg_rpm')
    expect(source).toContain('account.today.avg_tpm')
    expect(source).toContain("to='/accounts/$accountId/stats'")
    expect(source).toContain("to='/accounts/$accountId'")
  })

  test('uses grouped desktop columns and permanent detail actions', async () => {
    const source = await readFile(pagePath, 'utf8')

    expect(source).toContain("t('account.list.identity')")
    expect(source).toContain("t('account.list.ownership')")
    expect(source).toContain("t('account.list.todayUsage')")
    expect(source).toContain("t('site.dashboard.todayQuota')")
    expect(source).toContain("t('site.dashboard.todayTokens')")
    expect(source).toContain("t('site.dashboard.todayCount')")
    expect(source).not.toContain("t('site.dashboard.totalQuota')")
    expect(source).toContain("t('account.list.quota')")
    expect(source).toContain("t('account.list.dataStatus')")
    expect(source).toContain("t('account.actions.detail')")
    expect(source).toContain("t('account.actions.stats')")
  })
})
