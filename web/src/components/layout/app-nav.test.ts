import { describe, expect, test } from 'bun:test'

import { navGroups } from './app-nav-config'

describe('app navigation', () => {
  test('keeps platform users immediately above system settings', () => {
    const settingsGroup = navGroups.find(
      (group) => group.label === 'Settings and access'
    )

    expect(settingsGroup?.items.map((item) => item.to)).toEqual([
      '/settings/users',
      '/settings/system',
    ])
  })
})
