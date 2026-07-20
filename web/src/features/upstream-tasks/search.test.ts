import { describe, expect, test } from 'bun:test'

import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'

import { buildUpstreamTaskSearch } from './search'

describe('upstream task URL search', () => {
  test('preserves bigint-safe identities and normalizes frozen filters', () => {
    const search = buildUpstreamTaskSearch({
      actions: [' generate ', 'generate'],
      groups: ['vip', 'default'],
      models: ['safe-model'],
      platforms: ['video'],
      remoteChannelId: parseNonNegativeIdString('0'),
      remoteId: parseIdString('9007199254740993'),
      remoteUserId: parseNonNegativeIdString('9007199254740995'),
      siteIds: [parseIdString('9007199254740997')],
      statuses: ['SUCCESS', 'IN_PROGRESS', 'SUCCESS'],
      taskId: ' task-safe ',
    })
    expect(search.remoteId).toBe(parseIdString('9007199254740993'))
    expect(search.remoteChannelId).toBe(parseNonNegativeIdString('0'))
    expect(search.remoteUserId).toBe(
      parseNonNegativeIdString('9007199254740995')
    )
    expect(search.actions).toEqual(['generate'])
    expect(search.groups).toEqual(['default', 'vip'])
    expect(search.statuses).toEqual(['IN_PROGRESS', 'SUCCESS'])
    expect(search.taskId).toBe('task-safe')
  })

  test('defaults to no time boundary and fails closed for reversed ranges', () => {
    expect(buildUpstreamTaskSearch({}).start).toBeUndefined()
    expect(buildUpstreamTaskSearch({}).end).toBeUndefined()
    const search = buildUpstreamTaskSearch({ end: 10, start: 20 })
    expect(search.start).toBeUndefined()
    expect(search.end).toBeUndefined()
  })
})
