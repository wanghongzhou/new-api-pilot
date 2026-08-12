import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { isDashboardProblemSite } from './health'
import type { DashboardSiteHealthItem } from './types'

const healthySite: DashboardSiteHealthItem = {
  auth_status: 'authorized',
  health_status: 'ok',
  management_status: 'active',
  online_status: 'online',
  site_id: parseIdString('1'),
  site_name: 'Shanghai',
  statistics_status: 'ready',
  updated_at: 1,
}

describe('dashboard health presentation', () => {
  test('accepts the backend ready/ok states as healthy', () => {
    expect(isDashboardProblemSite(healthySite)).toBe(false)
  })

  test.each([
    ['management_status', 'disabled'],
    ['online_status', 'offline'],
    ['auth_status', 'expired'],
    ['statistics_status', 'partial'],
    ['health_status', 'warning'],
  ] as const)('marks %s=%s as requiring attention', (field, value) => {
    expect(isDashboardProblemSite({ ...healthySite, [field]: value })).toBe(
      true
    )
  })
})
