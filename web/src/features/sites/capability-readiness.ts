import { siteCapabilityKeys } from './constants'
import type { SiteCapabilityKey, SiteCapabilityResult } from './types'

export type CapabilityReadinessState = 'ready' | 'pending_config' | 'error'

export interface CapabilityReadinessIssue {
  key: SiteCapabilityKey
  result: SiteCapabilityResult | null
  status: 'failed' | 'unknown'
}

export interface CapabilityReadiness {
  contractValid: boolean
  issues: CapabilityReadinessIssue[]
  state: CapabilityReadinessState
}

const configurationCapabilityKeys = new Set<SiteCapabilityKey>([
  'data_export_enabled',
])

function isCapabilityKey(value: unknown): value is SiteCapabilityKey {
  return siteCapabilityKeys.includes(value as SiteCapabilityKey)
}

export function evaluateRequiredCapabilities(
  capabilities: readonly SiteCapabilityResult[]
): CapabilityReadiness {
  const results = new Map<SiteCapabilityKey, SiteCapabilityResult>()
  const duplicates = new Set<SiteCapabilityKey>()
  let contractValid = capabilities.length === siteCapabilityKeys.length

  for (const capability of capabilities) {
    if (!isCapabilityKey(capability.key)) {
      contractValid = false
      continue
    }
    if (results.has(capability.key)) {
      duplicates.add(capability.key)
      contractValid = false
      continue
    }
    results.set(capability.key, capability)
  }

  const issues: CapabilityReadinessIssue[] = []
  for (const key of siteCapabilityKeys) {
    const result = results.get(key) ?? null
    if (!result || duplicates.has(key)) {
      contractValid = false
      issues.push({ key, result: null, status: 'unknown' })
      continue
    }
    if (result.status === 'passed') continue
    if (key === 'flow_data_consistency' && result.status === 'skipped') {
      continue
    }
    issues.push({
      key,
      result,
      status: result.status === 'failed' ? 'failed' : 'unknown',
    })
  }

  if (contractValid && issues.length === 0) {
    return { contractValid: true, issues, state: 'ready' }
  }
  const configurationOnly =
    contractValid &&
    issues.length > 0 &&
    issues.every(
      (issue) =>
        issue.status === 'failed' && configurationCapabilityKeys.has(issue.key)
    )
  return {
    contractValid,
    issues,
    state: configurationOnly ? 'pending_config' : 'error',
  }
}
