import { describe, expect, test } from 'bun:test'

import {
  buildInventoryTrendChartValues,
  hasRenderableInventoryTrendValues,
  shouldShowInventoryTrendPoints,
  type InventoryTrendChartPoint,
} from './inventory-trend-chart-data'

const series = [{ key: 'count', label: 'Inventory' }]

function point(
  dataStatus: InventoryTrendChartPoint['dataStatus'],
  value: string
): InventoryTrendChartPoint {
  return {
    bucketStart: 1_754_352_000,
    dataStatus,
    values: { count: value },
  }
}

describe('inventory trend chart data semantics', () => {
  test('keeps complete and partial snapshot values', () => {
    expect(
      buildInventoryTrendChartValues(
        [point('complete', '2'), point('partial', '3')],
        series
      ).map((item) => item.value)
    ).toEqual([2, 3])
  })

  test('does not render unavailable collection states as real zero inventory', () => {
    expect(
      buildInventoryTrendChartValues(
        [
          point('missing', '0'),
          point('pending', '0'),
          point('unavailable', '0'),
          point('paused', '0'),
          point('backfilling', '0'),
          point('disabled', '0'),
        ],
        series
      ).map((item) => ({ rawValue: item.rawValue, value: item.value }))
    ).toEqual([
      { rawValue: null, value: null },
      { rawValue: null, value: null },
      { rawValue: null, value: null },
      { rawValue: null, value: null },
      { rawValue: null, value: null },
      { rawValue: null, value: null },
    ])
  })

  test('breaks the line for malformed values instead of coercing them', () => {
    expect(
      buildInventoryTrendChartValues([point('complete', '')], series)[0]
    ).toMatchObject({ rawValue: null, value: null })
    expect(
      buildInventoryTrendChartValues(
        [point('complete', 'not-a-number')],
        series
      )[0]
    ).toMatchObject({ rawValue: null, value: null })
    expect(
      buildInventoryTrendChartValues(
        [point('complete', 'Infinity'), point('partial', '1e999')],
        series
      ).map((item) => item.value)
    ).toEqual([null, null])
  })

  test('reports an empty chart when every snapshot value is null or invalid', () => {
    const values = buildInventoryTrendChartValues(
      [
        { ...point('complete', '1'), values: {} },
        point('complete', ''),
        point('partial', 'not-a-number'),
        point('unavailable', '4'),
      ],
      series
    )

    expect(hasRenderableInventoryTrendValues(values)).toBe(false)
  })

  test('reports a renderable chart when a complete or partial value is finite', () => {
    const completeValues = buildInventoryTrendChartValues(
      [point('complete', '0')],
      series
    )
    const partialValues = buildInventoryTrendChartValues(
      [point('partial', '3')],
      series
    )

    expect(hasRenderableInventoryTrendValues(completeValues)).toBe(true)
    expect(hasRenderableInventoryTrendValues(partialValues)).toBe(true)
  })

  test('only shows point markers for short ranges', () => {
    expect(shouldShowInventoryTrendPoints(48)).toBe(true)
    expect(shouldShowInventoryTrendPoints(49)).toBe(false)
    expect(shouldShowInventoryTrendPoints(720)).toBe(false)
  })
})
