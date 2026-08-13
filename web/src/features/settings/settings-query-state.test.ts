import { describe, expect, test } from 'bun:test'

const source = await Bun.file(
  new URL('./components/settings-page.tsx', import.meta.url)
).text()

describe('settings cached refresh failure boundary', () => {
  test('blocks only an initial failure and keeps cached failures read-only', () => {
    expect(source).toContain('settingsQuery.isError && !settingsQuery.data')
    expect(source).toContain(
      'settingsQuery.isRefetchError && Boolean(settingsQuery.data)'
    )
    expect(source).toContain('const canEdit = isAdmin && !cachedRefreshFailed')
    expect(source).toContain('isAdmin={canEdit}')
    expect(source).toContain("t('settings.staleWarning')")
  })
})
