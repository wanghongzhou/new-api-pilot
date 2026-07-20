import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import {
  canRetrySiteUsageRun,
  onboardingBackfillRunContractError,
  siteRunContractError,
  windowsBelongToSiteRun,
} from './run-contract'
import type { CollectionRunItem, CollectionRunWindowItem } from './types'

const run = {
  end_timestamp: 200,
  failed_windows: 1,
  site_id: '1',
  start_timestamp: 100,
  status: 'failed',
  target_id: '1',
  target_type: 'site',
  task_type: 'usage_backfill',
} as CollectionRunItem

describe('site collection run contract', () => {
  test('rejects a run from another site or target', () => {
    expect(siteRunContractError(run, '1')).toBeNull()
    expect(
      siteRunContractError({ ...run, site_id: parseIdString('2') }, '1')
    ).toBe('collection.contract.foreignRun')
    expect(
      siteRunContractError(
        { ...run, target_id: parseIdString('9'), target_type: 'account' },
        '1'
      )
    ).toBe('collection.contract.foreignRun')
  })

  test('only retries failed, ranged usage tasks owned by the site', () => {
    expect(canRetrySiteUsageRun(run, '1')).toBe(true)
    expect(canRetrySiteUsageRun({ ...run, task_type: 'site_probe' }, '1')).toBe(
      false
    )
    expect(canRetrySiteUsageRun({ ...run, start_timestamp: null }, '1')).toBe(
      false
    )
  })

  test('rejects windows that do not belong to both run and site', () => {
    const window = { run_id: '10', site_id: '1' } as CollectionRunWindowItem
    expect(windowsBelongToSiteRun([window], '10', '1')).toBe(true)
    expect(
      windowsBelongToSiteRun(
        [{ ...window, site_id: parseIdString('2') }],
        '10',
        '1'
      )
    ).toBe(false)
  })

  test('only accepts usage backfill runs returned by onboarding', () => {
    expect(onboardingBackfillRunContractError(run, '1')).toBeNull()
    expect(
      onboardingBackfillRunContractError(
        { ...run, task_type: 'resource_snapshot' },
        '1'
      )
    ).toBe('collection.contract.unexpectedOnboardingTask')
  })
})
