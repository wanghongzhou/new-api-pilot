import { describe, expect, test } from 'bun:test'

import { siteResourceColor } from './site-card-metrics'

describe('site card resource gradient', () => {
  test('uses capacity-oriented semantic anchors', () => {
    expect(siteResourceColor(null)).toBeUndefined()
    expect(siteResourceColor(0)).toContain(' 145)')
    expect(siteResourceColor(55)).toContain(' 105)')
    expect(siteResourceColor(75)).toContain(' 80)')
    expect(siteResourceColor(90)).toContain(' 50)')
    expect(siteResourceColor(100)).toContain(' 25)')
  })

  test('keeps low utilization green and interpolates every interval', () => {
    expect(siteResourceColor(27.5)).toContain(' 125)')
    expect(siteResourceColor(65)).toContain(' 92.5)')
    expect(siteResourceColor(82.5)).toContain(' 65)')
    expect(siteResourceColor(95)).toContain(' 37.5)')
  })

  test('clamps out-of-range percentages to the endpoints', () => {
    expect(siteResourceColor(-1)).toContain(' 145)')
    expect(siteResourceColor(101)).toContain(' 25)')
  })
})
