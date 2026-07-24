import { expect, test } from 'bun:test'

import { renderToStaticMarkup } from 'react-dom/server'

import '@/i18n/config'
import { parseMetricString } from '@/lib/api-types'

import { QuotaAmount } from './quota-amount'

const rate = {
  quota_per_unit: '500000',
  source: 'site' as const,
  updated_at: null,
  usd_exchange_rate: '7',
}

test('renders amount-only card values with primary metric emphasis', () => {
  const markup = renderToStaticMarkup(
    <QuotaAmount
      emphasizeAmount
      inline
      nullLabel='0'
      quota={parseMetricString('500000')}
      rate={rate}
      showQuota={false}
    />
  )
  expect(markup).toContain('text-base')
  expect(markup).toContain('font-semibold')
  expect(markup).toContain('$1.00 / ¥7.00')
})

test('renders an explicit zero when an amount-only quota is missing', () => {
  const markup = renderToStaticMarkup(
    <QuotaAmount
      emphasizeAmount
      inline
      nullLabel='0'
      quota={null}
      rate={rate}
      showQuota={false}
    />
  )
  expect(markup).toContain('>0</span>')
})
