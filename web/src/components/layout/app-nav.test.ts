import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

import { resolveAlertNavBadge } from './app-nav-badge'
import { navGroups } from './app-nav-config'

const appNavSource = readFileSync(
  new URL('./app-nav.tsx', import.meta.url),
  'utf8'
)
const routeTreeSource = readFileSync(
  new URL('../../routeTree.gen.ts', import.meta.url),
  'utf8'
)

describe('app navigation', () => {
  test('uses the approved concise Chinese navigation labels', () => {
    const locale = JSON.parse(
      readFileSync(
        new URL('../../i18n/locales/zh-CN.json', import.meta.url),
        'utf8'
      )
    ) as Record<string, string>

    expect(locale.Rankings).toBe('本地排行')
  })

  test('keeps zero, failed, and stale alert summaries distinct', () => {
    expect(resolveAlertNavBadge(0, false)).toEqual({ kind: 'none', text: null })
    expect(resolveAlertNavBadge(undefined, true)).toEqual({
      kind: 'unknown',
      text: '?',
    })
    expect(resolveAlertNavBadge(0, true)).toEqual({
      kind: 'stale',
      text: '!',
    })
    expect(resolveAlertNavBadge(7, true)).toEqual({
      kind: 'stale',
      text: '7',
    })
    expect(resolveAlertNavBadge(100, false)).toEqual({
      kind: 'count',
      text: '99+',
    })
  })

  test('orders navigation by common workflows', () => {
    expect(navGroups.map((group) => group.label)).toEqual([
      'Workspace',
      'Business management',
      'Tasks and logs',
      'Operations analytics',
      'Resource center',
      'Platform administration',
    ])
  })

  test('groups task and log workflows using native terminology', () => {
    const tasksGroup = navGroups.find(
      (group) => group.label === 'Tasks and logs'
    )

    expect(tasksGroup?.items.map(({ label, to }) => ({ label, to }))).toEqual([
      { label: 'Usage logs', to: '/logs' },
      { label: 'Task logs', to: '/upstream-tasks' },
      { label: 'System tasks', to: '/system-tasks' },
      { label: 'Export center', to: '/exports' },
    ])
  })

  test('uses a distinct icon for every navigation route', () => {
    const items = navGroups.flatMap((group) => group.items)
    expect(new Set(items.map((item) => item.to)).size).toBe(20)
    expect(new Set(items.map((item) => item.icon)).size).toBe(items.length)
  })

  test('keeps platform users immediately above system settings', () => {
    const settingsGroup = navGroups.find(
      (group) => group.label === 'Platform administration'
    )

    expect(settingsGroup?.items.map((item) => item.to)).toEqual([
      '/settings/users',
      '/settings/system',
    ])
  })

  test('uses native document navigation for every primary destination', () => {
    expect(appNavSource).toContain('href={item.to}')
    expect(appNavSource).not.toContain('<Link')
    expect(appNavSource).not.toContain('reloadDocument=')
  })

  test('keeps every primary route identity equal to its public URL', () => {
    for (const item of navGroups.flatMap((group) => group.items)) {
      expect(routeTreeSource).toContain(`fullPath: '${item.to}'`)
      expect(routeTreeSource).not.toContain(`fullPath: '${item.to}/'`)
    }
  })
})
