import { describe, expect, test } from 'bun:test'

import { displayNameSchema, loginSchema, passwordSchema } from './schema'

describe('authentication schemas', () => {
  test('normalizes a lowercase ASCII username', () => {
    const result = loginSchema.parse({
      password: 'Bootstrap123!',
      username: '  ADMIN  ',
    })
    expect(result.username).toBe('admin')
    expect(
      loginSchema.safeParse({
        password: 'Bootstrap123!',
        username: 'admin.user_01-test',
      }).success
    ).toBeTrue()
    expect(
      loginSchema.safeParse({
        password: 'Bootstrap123!',
        username: 'admin@example.com',
      }).success
    ).toBeFalse()
  })

  test('counts Unicode code points and enforces the bcrypt byte limit', () => {
    expect(passwordSchema.safeParse('七字符abc1').success).toBeFalse()
    expect(passwordSchema.safeParse('八个字符测试ab12').success).toBeTrue()
    expect(passwordSchema.safeParse('汉'.repeat(25)).success).toBeFalse()
  })

  test('counts display names by Unicode code point', () => {
    expect(displayNameSchema.safeParse('😀'.repeat(128)).success).toBeTrue()
    expect(displayNameSchema.safeParse('😀'.repeat(129)).success).toBeFalse()
  })
})
