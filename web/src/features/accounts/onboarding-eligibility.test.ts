import { describe, expect, test } from 'bun:test'

import type { CustomerListItem } from '@/features/customers/types'
import type { SiteListItem } from '@/features/sites/types'
import { parseIdString } from '@/lib/api-types'

import {
  isEligibleAccountCustomer,
  isEligibleAccountSite,
} from './onboarding-eligibility'

describe('account onboarding eligibility', () => {
  test('rejects a deep-linked customer that is no longer using', () => {
    const customer = {
      id: parseIdString('7'),
      status: 'disabled',
    } as CustomerListItem
    expect(isEligibleAccountCustomer([customer], '7')).toBeFalse()
  })

  test('requires an active authorized site with data export enabled', () => {
    const base = {
      auth_status: 'authorized',
      data_export_enabled: true,
      id: parseIdString('9'),
      management_status: 'active',
    } as SiteListItem
    expect(isEligibleAccountSite([base], '9')).toBeTrue()
    expect(
      isEligibleAccountSite(
        [{ ...base, data_export_enabled: false } as SiteListItem],
        '9'
      )
    ).toBeFalse()
  })
})
