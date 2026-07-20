import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import {
  collectionTaskCatalog,
  collectionTaskTypes,
  fastCollectionTaskTypes,
} from './constants'
import type {
  CollectionTaskCategory,
  CollectionTaskTriggerClass,
} from './constants'
import type { CollectionTaskType } from './types'

interface F12Task {
  category: CollectionTaskCategory
  purpose_key: `siteTasks.purpose.${string}`
  task_type: CollectionTaskType
  trigger_class: CollectionTaskTriggerClass
}

interface F12Fixture {
  fixture_id: 'F12'
  schema_version: 1
  tasks: F12Task[]
}

async function loadF12(): Promise<F12Fixture> {
  return JSON.parse(
    await readFile(
      new URL(
        '../../../../testdata/design/f12-site-task-catalog.json',
        import.meta.url
      ),
      'utf8'
    )
  ) as F12Fixture
}

describe('A101 F12 site task catalog consumption', () => {
  test('directly binds all nineteen task contracts to frontend metadata', async () => {
    const fixture = await loadF12()
    expect(fixture.fixture_id).toBe('F12')
    expect(fixture.tasks).toHaveLength(19)
    expect(fixture.tasks.map(({ task_type }) => task_type)).toEqual([
      ...collectionTaskTypes,
    ])

    for (const task of fixture.tasks) {
      expect(collectionTaskCatalog[task.task_type]).toMatchObject({
        category: task.category,
        purposeKey: task.purpose_key,
        triggerClass: task.trigger_class,
      })
    }

    expect(
      fixture.tasks
        .filter(({ category }) => category === 'fast')
        .map(({ task_type }) => task_type)
    ).toEqual([...fastCollectionTaskTypes])
  })
})
