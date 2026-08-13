import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

describe('upstream task production-ready responsive contract', () => {
  test('preserves the desktop safe fields in wide mobile cards', async () => {
    const page = await readFile(
      new URL('components/upstream-tasks-page.tsx', import.meta.url),
      'utf8'
    )

    expect(page).toContain('mobileScrollableContent')
    expect(page).toContain("mobileCardBreakpoint='wide'")
    expect(page).toContain("t('upstreamTasks.user')")
    expect(page).toContain("t('upstreamTasks.channel')")
    expect(page).toContain("t('upstreamTasks.submitValue'")
    expect(page).toContain("t('upstreamTasks.seenValue'")
    expect(page).toContain('common.retainedDataRefreshFailed')
    expect(page).toContain('common.siteOptionsRefreshFailed')
    expect(page).toContain('BigInt(list.total)')
    expect(page).toContain('paginationHasKnownLastPage={false}')
    expect(page).toContain(
      "<MetricValue value={list?.total ?? parseMetricString('0')} />"
    )
    expect(page).toContain("total={list?.total ?? '0'}")
  })
})
