import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

describe('logs mobile layout contract', () => {
  test('keeps the detail footer visible while the body scrolls', async () => {
    const source = await readFile(
      new URL('components/logs-page.tsx', import.meta.url),
      'utf8'
    )

    expect(source).toContain('max-h-[calc(100dvh-2rem)] max-w-3xl')
    expect(source).toContain(
      "className='grid min-h-0 gap-4 overflow-y-auto pr-1'"
    )
    expect(source).toMatch(
      /overflow-y-auto pr-1'[\s\S]*<\/div>\s*<DialogFooter>/
    )
    expect(source).toContain('mobileScrollableContent')
    expect(source).toContain("mobileCardBreakpoint='wide'")
    expect(source).toContain(
      "import { isConsumptionLogType } from '../display'"
    )
    expect(
      source.match(/isConsumptionLogType\(/g)?.length
    ).toBeGreaterThanOrEqual(8)
    expect(source).toContain('common.retainedDataRefreshFailed')
    expect(source).toContain('common.siteOptionsRefreshFailed')
    expect(source).toContain('buildLogQuickRange(value as LogQuickRange)')
    expect(source).toContain('const quickRange = getLogQuickRange(search)')
    expect(source).toContain("quickRange === 'custom'")
    expect(source).toContain('value={quickRange}')
    const mobileCard = source.slice(
      source.indexOf('renderMobileCard={(item) => ('),
      source.indexOf('total={0}')
    )
    expect(mobileCard).toContain('isConsumptionLogType(item.type)')
    expect(mobileCard).toContain("t('logs.fields.mode')")
    expect(mobileCard).toContain("t('logs.fields.tokens')")
    expect(mobileCard).toContain("t('logs.fields.cost')")
    expect(mobileCard).toContain("t('logs.fields.timing')")
    expect(mobileCard).toContain('<LogTokenUsage item={item} />')
    expect(mobileCard).toContain(
      '<LogCost quota={item.quota} rate={item.rate} />'
    )
    expect(mobileCard).toContain('formatTokensPerSecond(')
    expect(mobileCard).toContain("<LogTiming indicator='dot' item={item} />")
    expect(source).toContain(
      "className='text-muted-foreground/50 text-[11px] leading-none'"
    )
    expect(source).toContain("t('logs.cost.cny', { amount: cny })")
    expect(source).toContain("t('logs.cost.usd', { amount: usd })")
    expect(source).toContain("success: 'text-success'")
    expect(source).toContain("warning: 'text-warning'")
    expect(source).toContain("danger: 'text-destructive'")
    expect(source).toContain("t('logs.stats.usage')")
    expect(source).toContain("t('logs.stats.rpm')")
    expect(source).toContain("t('logs.stats.tpm')")
    expect(source).toContain('onSearch={() => void logsQuery.refetch()}')
    expect(source).toContain("{t('common.search')}")
    expect(source).toContain('BigInt(data.total)')
    expect(source).toContain('paginationHasKnownLastPage={false}')
    expect(source).toContain(
      "<MetricValue value={data?.total ?? parseMetricString('0')} />"
    )
    const username = source.indexOf("aria-label={t('logs.fields.username')}")
    const model = source.indexOf("aria-label={t('logs.fields.model')}")
    const channel = source.indexOf("aria-label={t('logs.fields.channelId')}")
    const site = source.indexOf("title={t('logs.filters.site')}")
    expect(username).toBeGreaterThan(-1)
    expect(model).toBeGreaterThan(username)
    expect(channel).toBeGreaterThan(model)
    expect(site).toBeGreaterThan(channel)
  })
})
