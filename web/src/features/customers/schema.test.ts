import { describe, expect, test } from 'bun:test'

import { customerFormSchema, customersSearchSchema } from './schema'

describe('customer schemas', () => {
  test('restores a single status filter as an array', () => {
    expect(customersSearchSchema.parse({ status: 'using' }).status).toEqual([
      'using',
    ])
  })

  test('trims profile fields and rejects disabled through the generic form', () => {
    expect(
      customerFormSchema.parse({
        contact: ' 张经理 ',
        name: ' 示例客户 ',
        remark: ' 重点客户 ',
        status: 'using',
      })
    ).toEqual({
      contact: '张经理',
      name: '示例客户',
      remark: '重点客户',
      status: 'using',
    })
    expect(
      customerFormSchema.safeParse({
        contact: '',
        name: '示例客户',
        remark: '',
        status: 'disabled',
      }).success
    ).toBeFalse()
  })
})
