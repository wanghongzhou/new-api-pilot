import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const pagePath = new URL(
  './components/channel-inventory-page.tsx',
  import.meta.url
)

describe('channel inventory numeric presentation', () => {
  test('formats bigint, decimal, and ratio values by their actual semantics', async () => {
    const source = await readFile(pagePath, 'utf8')

    expect(source).toContain('formatMetricDisplayValue')
    expect(source).toContain('formatDecimalDisplayValue')
    expect(source).toContain('availabilityText')
    expect(source).toContain('new Decimal(value).mul(100)')
    expect(source).not.toContain('value: row.original.used_quota,')
    expect(source).not.toContain('<dd>{item.response_time_ms}</dd>')
    expect(source).not.toContain("accessorKey: 'availability_rate'")
  })
})
