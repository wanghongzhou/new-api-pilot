import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

const source = readFileSync(
  new URL('./components/statistics-filters.tsx', import.meta.url),
  'utf8'
)

describe('statistics flow filter contract', () => {
  test('keeps group and node choices value-scoped and encodes empty identities', () => {
    expect(source).toContain('encodeStatisticsDimensionFilter(group.use_group)')
    expect(source).toContain('encodeStatisticsDimensionFilter(node.node_name)')
    expect(source).not.toContain("t('statistics.filter.groupOption'")
    expect(source).not.toContain("t('statistics.filter.nodeOption'")
  })
})
