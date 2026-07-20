import { describe, expect, test } from 'bun:test'

import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'

import { buildUpstreamTaskExportRequest } from './export-request'
import { buildUpstreamTaskSearch } from './search'

describe('upstream task export request', () => {
  test('freezes all safe filters including channel identity without pagination', () => {
    const request = buildUpstreamTaskExportRequest(
      'xlsx',
      buildUpstreamTaskSearch({
        actions: ['generate'],
        end: 1_784_348_800,
        groups: ['default'],
        models: ['safe-model'],
        page: 9,
        platforms: ['video'],
        remoteChannelId: parseNonNegativeIdString('0'),
        remoteId: parseIdString('9007199254740993'),
        remoteUserId: parseNonNegativeIdString('9007199254740995'),
        siteIds: [parseIdString('9007199254740997')],
        start: 1_784_262_400,
        statuses: ['IN_PROGRESS'],
        taskId: 'task-safe',
      })
    )
    expect(request.statistics_type).toBe('upstream_tasks')
    expect(request.filters.remote_channel_id).toBe(
      parseNonNegativeIdString('0')
    )
    expect(request.filters.task_statuses).toEqual(['IN_PROGRESS'])
    expect(JSON.stringify(request)).not.toContain('page_size')
    const serialized = JSON.stringify(request).toLowerCase()
    const forbidden = [
      ['in', 'put'].join(''),
      ['fail', 'reason'].join('_'),
      ['result', 'url'].join('_'),
      ['private', 'data'].join('_'),
      ['down', 'load', 'url'].join('_'),
    ]
    for (const field of forbidden) expect(serialized).not.toContain(field)
  })
})
