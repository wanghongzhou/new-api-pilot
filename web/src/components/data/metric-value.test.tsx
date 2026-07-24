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
  test('renders missing numeric values as zero by default', () => {
    expect(renderToStaticMarkup(<MetricValue value={null} />)).toContain('0')
  })

  test('preserves numeric values and normalizes null to zero', () => {
    expect(
      renderToStaticMarkup(<MetricValue value={parseMetricString('1')} />)
    ).toContain('title="1">1</span>')
    expect(
      renderToStaticMarkup(<MetricValue value={parseMetricString('0')} />)
    ).toContain('title="0">0</span>')
    expect(renderToStaticMarkup(<MetricValue value={null} />)).toContain(
      '>0</span>'
    )
  })
})
