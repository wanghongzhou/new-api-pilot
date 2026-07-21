import { describe, expect, test } from 'bun:test'

import { customerFormSchema, customersSearchSchema } from './schema'

describe('customer schemas', () => {
  test('restores a single status filter as an array', () => {
    expect(customersSearchSchema.parse({ status: 'using' }).status).toEqual([
      'using',
    ])
  })

  test('trims profile fields and validates amounts', () => {
    expect(
      customerFormSchema.parse({
        contact: ' contact ',
        contract_amount: '123.45',
        name: ' customer ',
        payment_amount: '23.45',
        remark: ' remark ',
        status: 'using',
      })
    ).toEqual({
      contact: 'contact',
      contract_amount: '123.45',
      name: 'customer',
      payment_amount: '23.45',
      remark: 'remark',
      status: 'using',
    })
    expect(
      customerFormSchema.safeParse({
        contact: '',
        contract_amount: '-1',
        name: 'customer',
        payment_amount: '1.12345678901',
        remark: '',
        status: 'using',
      }).success
    ).toBeFalse()
  })
})
