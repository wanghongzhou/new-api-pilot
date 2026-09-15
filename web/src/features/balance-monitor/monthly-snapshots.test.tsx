import { expect, test } from 'bun:test'

import { createInstance } from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import zh from '@/i18n/locales/zh-CN.json'

import type { RecordItem } from './data'
import { MonthlyReportView } from './monthly-snapshots'

const i18n = createInstance()
await i18n.init({
  lng: 'zh-CN',
  resources: { 'zh-CN': { translation: zh } },
  interpolation: { escapeValue: false },
})

test('monthly result preserves saved values and has no consumption columns', () => {
  const row = {
    sampled_at: '1788192000',
    monthly: {
      period: '2026-08',
      mode: 'legacy',
      grand_total_residual_yuan: '1290',
      total_excluding_nowcoding_residual_yuan: '1240',
      foxcode_residual_yuan: '410',
      other_residual_yuan: '20',
      nowcoding_residual_yuan: '50',
      redeem_code_residual_yuan: '810',
      redeem_code_count: '2',
      accounts: [{ name: 'fox', raw: '2000000000', residual_yuan: '410' }],
      failed: ['offline'],
    },
  } as RecordItem
  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <MonthlyReportView row={row} />
    </I18nextProvider>
  )
  expect(html).toContain('2026-08 余额快照')
  expect(html).toContain('1290.0000')
  expect(html).toContain('810.0000')
  expect(html).toContain('410.0000')
  expect(html).not.toContain('425.0000')
  expect(html).toContain('offline')
  expect(html).toContain('历史文件导入')
  expect(html).not.toContain('充值修正后消耗')
  expect(html).not.toContain('两次巡检时间')
})
