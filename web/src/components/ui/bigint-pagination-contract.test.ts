import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const paginationPath = new URL('./data-table-pagination.tsx', import.meta.url)

describe('PageData bigint pagination contract', () => {
  test('keeps totals exact and only exposes a last page inside the safe integer range', async () => {
    const source = await readFile(paginationPath, 'utf8')

    expect(source).toContain(
      'BigInt(currentPage) * BigInt(pageSize) < totalCount'
    )
    expect(source).toContain('formatMetricDisplayValue(String(total))')
    expect(source).toContain(
      'totalPagesBigInt <= BigInt(Number.MAX_SAFE_INTEGER)'
    )
    expect(source).toContain('totalCount <= BigInt(Number.MAX_SAFE_INTEGER)')
    expect(source).toContain('{totalPages && (')
    expect(source).not.toContain('Math.ceil(total / pageSize)')
  })
})
