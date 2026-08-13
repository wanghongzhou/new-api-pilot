import { describe, expect, test } from 'bun:test'

import { renderToStaticMarkup } from 'react-dom/server'

import '@/i18n/config'
import { parseMetricString } from '@/lib/api-types'

import { MetricValue } from './metric-value'

test('supports an explicit zero fallback when the metric contract defines it', () => {
  expect(
    renderToStaticMarkup(<MetricValue nullLabel='0' value={null} />)
  ).toContain('>0</span>')
})

describe('MetricValue', () => {
  test('renders missing numeric values with the shared placeholder by default', () => {
    expect(renderToStaticMarkup(<MetricValue value={null} />)).toContain(
      '>-</span>'
    )
  })

  test('preserves real zero without treating missing values as zero', () => {
    expect(
      renderToStaticMarkup(<MetricValue value={parseMetricString('1')} />)
    ).toContain('title="1">1</span>')
    expect(
      renderToStaticMarkup(<MetricValue value={parseMetricString('0')} />)
    ).toContain('title="0">0</span>')
    expect(renderToStaticMarkup(<MetricValue value={null} />)).toContain(
      '>-</span>'
    )
  })

  test('groups bigint values without losing the authoritative string', () => {
    const value = parseMetricString('9007199254740993123456789')
    const markup = renderToStaticMarkup(<MetricValue value={value} />)

    expect(markup).toContain('title="9007199254740993123456789"')
    expect(markup).toContain('>9,007,199,254,740,993,123,456,789</span>')
  })
})
