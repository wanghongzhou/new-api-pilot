import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import {
  collectionTaskCatalog,
  collectionTaskTypes,
  fastCollectionTaskTypes,
  isFastCollectionTaskType,
} from './constants'
import { siteRunContractError } from './run-contract'
import { siteDetailSearchSchema } from './schema'
import type { CollectionRunItem, CollectionTaskType } from './types'

const baseRun = {
  site_id: '1',
  target_id: '1',
  target_type: 'site',
} as CollectionRunItem

describe('A101 frontend site task catalog', () => {
  test('accepts, filters, and labels all nineteen authoritative types', async () => {
    expect(collectionTaskTypes).toHaveLength(19)
    const locale = JSON.parse(
      await readFile(
        new URL('../../i18n/locales/zh-CN.json', import.meta.url),
        'utf8'
      )
    ) as Record<string, string>

    for (const taskType of collectionTaskTypes) {
      const metadata = collectionTaskCatalog[taskType]
      expect(locale[`collection.task.${taskType}`]?.trim()).not.toBe('')
      expect(locale[metadata.purposeKey]?.trim()).not.toBe('')
      expect(
        siteDetailSearchSchema.parse({ runTaskType: taskType }).runTaskType
      ).toBe(taskType)

      const run = {
        ...baseRun,
        target_id: metadata.targetType === 'site' ? '1' : '99',
        target_type: metadata.targetType,
        task_type: taskType,
      } as CollectionRunItem
      expect(siteRunContractError(run, '1')).toBeNull()
    }
  })

  test('keeps fast history to its exact three types and blocks unknown values', () => {
    expect(fastCollectionTaskTypes).toEqual([
      'site_probe',
      'realtime_stat',
      'resource_snapshot',
    ])
    for (const taskType of fastCollectionTaskTypes) {
      expect(isFastCollectionTaskType(taskType)).toBe(true)
    }
    expect(isFastCollectionTaskType('performance_sync')).toBe(false)
    expect(isFastCollectionTaskType('unknown_task')).toBe(false)

    expect(
      siteRunContractError(
        { ...baseRun, task_type: 'unknown_task' as CollectionTaskType },
        '1'
      )
    ).toBe('collection.contract.unknownTaskType')
  })
})
