import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { evaluateRequiredCapabilities } from './capability-readiness'
import { siteCapabilityKeys } from './constants'
import type { SiteCapabilityKey, SiteCapabilityResult } from './types'

function capabilities(
  overrides: Partial<
    Record<SiteCapabilityKey, SiteCapabilityResult['status']>
  > = {}
): SiteCapabilityResult[] {
  return siteCapabilityKeys.map((key) => ({
    key,
    message: {
      code: 'CAPABILITY_OK',
      params: { capability_key: key, site_id: parseIdString('1') },
      technical_detail: '',
    },
    status: overrides[key] ?? 'passed',
  }))
}

describe('required site capabilities', () => {
  test('accepts all passed capabilities and the documented no-traffic skip', () => {
    expect(evaluateRequiredCapabilities(capabilities()).state).toBe('ready')
    expect(
      evaluateRequiredCapabilities(
        capabilities({ flow_data_consistency: 'skipped' })
      ).state
    ).toBe('ready')
  })

  test('classifies export failures as pending configuration', () => {
    const result = evaluateRequiredCapabilities(
      capabilities({ data_export_enabled: 'failed' })
    )
    expect(result.state).toBe('pending_config')
    expect(result.issues.map((issue) => issue.key)).toEqual([
      'data_export_enabled',
    ])
  })

  test('classifies other required failures as errors', () => {
    const result = evaluateRequiredCapabilities(
      capabilities({ instance_contract: 'failed' })
    )
    expect(result.state).toBe('error')
    expect(result.issues[0]?.status).toBe('failed')
  })

  test('treats missing, duplicate, and invalid required states as unknown', () => {
    const missing = capabilities().slice(1)
    expect(evaluateRequiredCapabilities(missing)).toMatchObject({
      contractValid: false,
      state: 'error',
    })

    const complete = capabilities()
    const first = complete[0]
    if (!first) throw new Error('capability fixture is empty')
    const duplicate = [...complete, first]
    expect(evaluateRequiredCapabilities(duplicate).issues[0]).toMatchObject({
      key: 'status_contract',
      status: 'unknown',
    })

    const invalidSkip = capabilities({ instance_contract: 'skipped' })
    expect(evaluateRequiredCapabilities(invalidSkip).issues[0]).toMatchObject({
      key: 'instance_contract',
      status: 'unknown',
    })
  })
})
