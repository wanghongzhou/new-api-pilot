import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

import {
  collectionTaskHasWindows,
  windowedCollectionTaskTypes,
} from './constants'

const collectionRunsSource = readFileSync(
  new URL('./components/collection-runs-panel.tsx', import.meta.url),
  'utf8'
)
const fastHistorySource = readFileSync(
  new URL('./components/fast-task-history-panel.tsx', import.meta.url),
  'utf8'
)
const siteConstantsSource = readFileSync(
  new URL('./constants.ts', import.meta.url),
  'utf8'
)
const dataTableSource = readFileSync(
  new URL('../../components/ui/data-table.tsx', import.meta.url),
  'utf8'
)

describe('site detail collection pagination', () => {
  test('keeps long run and window histories discoverably paginated', () => {
    expect(collectionRunsSource).toContain('const collectionRunPageSize = 10')
    expect(collectionRunsSource).toContain(
      'const collectionWindowPageSize = 10'
    )
    expect(collectionRunsSource.match(/<DataTablePagination/g)).toHaveLength(1)
    expect(collectionRunsSource).toContain('fillAvailableHeight={false}')
    expect(collectionRunsSource).toContain('paginationInFooter={false}')
    expect(collectionRunsSource).toContain(
      'flex flex-wrap items-end justify-between gap-3'
    )
    expect(collectionRunsSource).toContain("className='w-32'")
    expect(collectionRunsSource).toContain("className='w-48'")
    expect(collectionRunsSource).toContain("className='min-w-0 flex-1'")
    expect(collectionRunsSource).toContain('firstOffset >= BigInt(runTotal)')
    expect(collectionRunsSource).toContain('firstOffset >= BigInt(windowTotal)')
  })

  test('limits fast task history and recovers from an out-of-range page', () => {
    expect(fastHistorySource).toContain('const fastTaskHistoryPageSize = 10')
    expect(fastHistorySource).not.toContain('<DataTablePagination')
    expect(fastHistorySource).toContain('fillAvailableHeight={false}')
    expect(fastHistorySource).toContain('paginationInFooter={false}')
    expect(fastHistorySource).toContain(
      'flex flex-wrap items-end justify-between gap-3'
    )
    expect(fastHistorySource).toContain("className='w-32'")
    expect(fastHistorySource).toContain("className='w-48'")
    expect(fastHistorySource).toContain("className='min-w-0 flex-1'")
    expect(fastHistorySource).toContain('renderMobileCard={(item) =>')
    expect(fastHistorySource).not.toContain('border-t pt-5')
    expect(fastHistorySource).toContain(
      'if (search.fastPage > lastPage) onSearchChange({ fastPage: lastPage })'
    )
  })

  test('leaves long-page vertical scrolling to the page content area', () => {
    expect(dataTableSource).toContain(
      "? 'h-full overflow-auto overscroll-contain'"
    )
    expect(dataTableSource).toContain(": 'overflow-x-auto overflow-y-hidden'")
    expect(dataTableSource).toContain(": 'overflow-visible min-[641px]:hidden'")
  })

  test('offers execution windows only for the task types supported by the backend', () => {
    expect(windowedCollectionTaskTypes).toEqual([
      'usage_hour',
      'usage_backfill',
      'usage_validation',
      'account_rebuild',
      'customer_rebuild',
    ])
    expect(collectionTaskHasWindows('usage_hour')).toBe(true)
    expect(collectionTaskHasWindows('system_task_sync')).toBe(false)
    expect(siteConstantsSource).toContain('windowedCollectionTaskTypes')
    expect(collectionRunsSource).toContain(
      'collectionTaskHasWindows(row.original.task_type)'
    )
    expect(collectionRunsSource).toContain(
      'enabled: validRunId && validSiteId && windowedRun'
    )
  })

  test('does not render an empty technical-detail block for safe task failures', () => {
    expect(collectionRunsSource).toContain('run.error.technical_detail && (')
    expect(collectionRunsSource).toContain('run.last_request_id && (')
  })
})
