import {
  collectionTaskCatalog,
  collectionTaskTypes,
  retryableSiteUsageTaskTypes,
} from './constants'
import type {
  CollectionRunItem,
  CollectionRunWindowItem,
  CollectionTaskType,
} from './types'

export type SiteRunContractError =
  | 'collection.contract.foreignRun'
  | 'collection.contract.unexpectedOnboardingTask'
  | 'collection.contract.unknownTaskType'

export function isCollectionTaskType(
  value: unknown
): value is CollectionTaskType {
  return collectionTaskTypes.includes(value as CollectionTaskType)
}

export function siteRunContractError(
  run: CollectionRunItem,
  siteId: string
): SiteRunContractError | null {
  if (!isCollectionTaskType(run.task_type)) {
    return 'collection.contract.unknownTaskType'
  }
  const targetType = collectionTaskCatalog[run.task_type].targetType
  if (
    run.site_id !== siteId ||
    run.target_type !== targetType ||
    (targetType === 'site' && run.target_id !== siteId)
  ) {
    return 'collection.contract.foreignRun'
  }
  return null
}

export function windowsBelongToSiteRun(
  windows: readonly CollectionRunWindowItem[],
  runId: string,
  siteId: string
): boolean {
  return windows.every(
    (window) => window.run_id === runId && window.site_id === siteId
  )
}

export function onboardingBackfillRunContractError(
  run: CollectionRunItem,
  siteId: string
): SiteRunContractError | null {
  const contractError = siteRunContractError(run, siteId)
  if (contractError) return contractError
  return run.task_type === 'usage_backfill'
    ? null
    : 'collection.contract.unexpectedOnboardingTask'
}

export function canRetrySiteUsageRun(
  run: CollectionRunItem,
  siteId: string
): boolean {
  return (
    siteRunContractError(run, siteId) == null &&
    retryableSiteUsageTaskTypes.includes(
      run.task_type as (typeof retryableSiteUsageTaskTypes)[number]
    ) &&
    run.status === 'failed' &&
    run.failed_windows > 0 &&
    run.start_timestamp != null &&
    run.end_timestamp != null
  )
}
