import { describe, expect, test } from 'bun:test'

import { renderToStaticMarkup } from 'react-dom/server'

import '@/i18n/config'
import { parseMetricString } from '@/lib/api-types'

import { MetricValue } from './metric-value'

test('renders a supplied null label without coercing it to zero', () => {
  expect(
    renderToStaticMarkup(<MetricValue nullLabel='—' value={null} />)
  ).toContain('>—</span>')
})

describe('MetricValue', () => {
  test('preserves the account active-user 1, 0, and null states', () => {
    expect(
      renderToStaticMarkup(<MetricValue value={parseMetricString('1')} />)
    ).toContain('title="1">1</span>')
    expect(
      renderToStaticMarkup(<MetricValue value={parseMetricString('0')} />)
    ).toContain('title="0">0</span>')
    expect(renderToStaticMarkup(<MetricValue value={null} />)).toContain(
      '>不可用</span>'
    )
  })
})
