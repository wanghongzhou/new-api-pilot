import { describe, expect, test } from 'bun:test'

import { accountOnboardingSchema, accountsSearchSchema } from './schema'

describe('account schemas', () => {
  test('uses snake-case deep-link IDs and restores multi filters', () => {
    expect(
      accountsSearchSchema.parse({
        customer_id: '9007199254740993',
        remoteState: 'missing',
        site_id: '9007199254740994',
      })
    ).toMatchObject({
      customer_id: '9007199254740993',
      remoteState: ['missing'],
      site_id: '9007199254740994',
    })
  })

  test('requires positive string IDs and explicit immutable-binding confirmation', () => {
    const values = {
      bindingConfirmed: false,
      customerId: '9007199254740993',
      remark: '',
      remoteUserId: '9007199254740995',
      siteId: '9007199254740994',
    }
    expect(accountOnboardingSchema.safeParse(values).success).toBeFalse()
    expect(
      accountOnboardingSchema.safeParse({
        ...values,
        bindingConfirmed: true,
      }).success
    ).toBeTrue()
    expect(
      accountOnboardingSchema.safeParse({
        ...values,
        bindingConfirmed: true,
        remoteUserId: '0',
      }).success
    ).toBeFalse()
  })
})
