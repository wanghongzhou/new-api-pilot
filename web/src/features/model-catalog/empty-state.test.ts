import { describe, expect, test } from 'bun:test'

import {
  getMissingModelEmptyState,
  getModelCatalogEmptyState,
} from './empty-state'
import { buildModelCatalogSearch } from './search'

describe('model catalog empty state', () => {
  test('explains a successfully collected empty upstream catalog', () => {
    expect(
      getModelCatalogEmptyState('complete', buildModelCatalogSearch({}))
    ).toEqual({
      descriptionKey: 'modelCatalog.empty.upstreamDescription',
      titleKey: 'modelCatalog.empty.upstreamTitle',
    })
  })

  test('does not describe filtered results as an empty upstream catalog', () => {
    expect(
      getModelCatalogEmptyState(
        'complete',
        buildModelCatalogSearch({ keyword: 'gpt' })
      )
    ).toEqual({
      descriptionKey: 'modelCatalog.empty.filteredDescription',
      titleKey: 'modelCatalog.empty.filteredTitle',
    })
  })

  test.each([
    ['pending', 'pending'],
    ['partial', 'partial'],
    ['unavailable', 'unavailable'],
  ] as const)('keeps %s collection state explicit', (status, key) => {
    expect(
      getModelCatalogEmptyState(status, buildModelCatalogSearch({}))
    ).toEqual({
      descriptionKey: `modelCatalog.empty.${key}Description`,
      titleKey: `modelCatalog.empty.${key}Title`,
    })
  })
})

describe('missing model empty state', () => {
  test('distinguishes a complete zero result from incomplete collection', () => {
    expect(
      getMissingModelEmptyState('complete', buildModelCatalogSearch({}))
    ).toEqual({
      descriptionKey: 'modelCatalog.missingEmpty.completeDescription',
      titleKey: 'modelCatalog.missingEmpty.completeTitle',
    })
    expect(
      getMissingModelEmptyState('unavailable', buildModelCatalogSearch({}))
    ).toEqual({
      descriptionKey: 'modelCatalog.missingEmpty.unavailableDescription',
      titleKey: 'modelCatalog.missingEmpty.unavailableTitle',
    })
  })

  test('uses a filtered empty result when the user is searching', () => {
    expect(
      getMissingModelEmptyState(
        'complete',
        buildModelCatalogSearch({ keyword: 'gpt' })
      )
    ).toEqual({
      descriptionKey: 'modelCatalog.missingEmpty.filteredDescription',
      titleKey: 'modelCatalog.missingEmpty.filteredTitle',
    })
  })
})
