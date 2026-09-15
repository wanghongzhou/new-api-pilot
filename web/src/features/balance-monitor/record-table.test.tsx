import { describe, expect, test } from 'bun:test'

import { createInstance } from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import zh from '@/i18n/locales/zh-CN.json'

import type { BalanceSearch, RecordItem } from './data'
import { BalanceRecordTable } from './record-table'

const i18n = createInstance()
await i18n.init({
  lng: 'zh-CN',
  resources: { 'zh-CN': { translation: zh } },
  interpolation: { escapeValue: false },
})
const row: RecordItem = {
  source_id: 'source',
  record_id: 'record',
  revision: '1',
  kind: 'current',
  account_id: 'account',
  site_id: 'site',
  name: '测试账号',
  site_name: 'test.invalid',
  day: '2026-09-15',
  sampled_at: '1789459200',
  started_at: '1789457400',
  received_at: '1789459201',
  balance: '1000',
  previous_balance: '1100',
  balance_delta: '100',
  consumption: '100',
  rate: '1',
  consumption_yuan: '100',
  balance_yuan: '1000',
  field: 'quota',
  flags: [],
  coverage_seconds: '43200',
}
function render(kind: Exclude<BalanceSearch['kind'], 'monthly'>) {
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <BalanceRecordTable
        kind={kind}
        rows={[{ ...row, kind }]}
        onDetail={() => {}}
        onHistory={() => {}}
      />
    </I18nextProvider>
  )
}
describe('balance view separation', () => {
  test('current view contains balances but no consumption measurements', () => {
    const html = render('current')
    expect(html).toContain('最新余额（元）')
    expect(html).toContain('余额更新时间')
    expect(html).not.toContain('充值修正后消耗（元）')
    expect(html).not.toContain('两次巡检时间')
    expect(html.match(/<table/g)).toHaveLength(1)
  })
  test('interval view shows the actual window and comparison, without a realtime balance column', () => {
    const html = render('interval')
    expect(html).toContain('两次巡检时间')
    expect(html).toContain('30.0 分钟')
    expect(html).toContain('余额差额（元）')
    expect(html).toContain('充值修正后消耗（元）')
    expect(html).not.toContain('最新余额（元）')
    expect(html).not.toContain('原始余额')
  })
  test('daily view shows date, daily consumption and completeness', () => {
    const html = render('daily')
    expect(html).toContain('报告日期')
    expect(html).toContain('当日估算消耗（元）')
    expect(html).toContain('50.0%')
    expect(html).not.toContain('两次巡检时间')
    expect(html).not.toContain('最新余额（元）')
  })
})
