import { describe, expect, test } from 'bun:test'

import { renderToStaticMarkup } from 'react-dom/server'

import '@/i18n/config'
import zhCN from '@/i18n/locales/zh-CN.json'

import { StatisticsSummary } from './components/entity-statistics'
import type { StatisticsResponse, StatisticsSearch } from './types'

const search = {
  display: 'quota',
} as StatisticsSearch

const data = {
  range: {
    as_of: 1_700_000_000,
  },
  summary: {
    data_status: 'complete',
    is_partial: false,
    request_count: '1',
    quota: '2',
    token_used: '3',
    active_users: '1',
  },
  scope: 'global',
  site_breakdown: [],
} as unknown as StatisticsResponse

describe('statistics freshness copy', () => {
  test('states that the as-of value is the latest complete hour', () => {
    expect(zhCN['statistics.asOf']).toContain('最近完整小时')
  })

  test('renders the settled-hour comparison notice in the summary', () => {
    const markup = renderToStaticMarkup(
      <StatisticsSummary data={data} search={search} />
    )
    expect(markup).toContain('平台统计仅包含已结算')
    expect(markup).toContain('源站当前小时仍会增长')
  })
})
