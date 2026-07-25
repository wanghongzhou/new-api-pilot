import { describe, expect, test } from 'bun:test'

import {
  createPlatformUserSchema,
  editPlatformUserSchema,
  platformUserSearchSchema,
  resetPlatformUserPasswordSchema,
} from './schema'

describe('platform user form schemas', () => {
  test('enforces the backend username, display name, role, and password contract', () => {
    expect(
      createPlatformUserSchema.safeParse({
        confirmPassword: 'temporary-pass',
        displayName: 'Operator',
        password: 'temporary-pass',
        role: 'viewer',
        username: 'operator.one',
      }).success
    ).toBeTrue()

    for (const candidate of [
      { username: 'bad user' },
      { displayName: '' },
      { role: 'owner' },
      { password: 'short', confirmPassword: 'short' },
      { confirmPassword: 'different' },
    ]) {
      expect(
        createPlatformUserSchema.safeParse({
          confirmPassword: 'temporary-pass',
          displayName: 'Operator',
          password: 'temporary-pass',
          role: 'viewer',
          username: 'operator.one',
          ...candidate,
        }).success
      ).toBeFalse()
    }
  })

  test('validates edits, resets, and bounded URL search state', () => {
    expect(
      editPlatformUserSchema.safeParse({
        displayName: 'Administrator',
        role: 'admin',
        username: 'admin',
      }).success
    ).toBeTrue()
    expect(
      resetPlatformUserPasswordSchema.safeParse({
        confirmPassword: 'replacement-pass',
        password: 'replacement-pass',
      }).success
    ).toBeTrue()
    expect(
      resetPlatformUserPasswordSchema.safeParse({
        confirmPassword: 'different-pass',
        password: 'replacement-pass',
      }).success
    ).toBeFalse()

    const search = platformUserSearchSchema.parse({
      page: '-1',
      pageSize: '1000',
      role: 'owner',
      status: '3',
    })
    expect(search.page).toBeUndefined()
    expect(search.pageSize).toBeUndefined()
    expect(search.role).toBeUndefined()
    expect(search.status).toBeUndefined()
    expect(search.order).toBeUndefined()
    expect(search.sort).toBeUndefined()
  })
})
