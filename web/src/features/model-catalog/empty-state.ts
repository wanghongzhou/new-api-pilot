import type { DataStatus } from '@/lib/api-types'

import type { ModelCatalogSearch } from './search'

export interface ModelCatalogEmptyState {
  descriptionKey: string
  titleKey: string
}

function hasContentFilter(search: ModelCatalogSearch) {
  return (
    search.keyword !== '' ||
    search.vendorId !== undefined ||
    search.statuses.length > 0 ||
    search.syncOfficial.length > 0
  )
}

export function getModelCatalogEmptyState(
  dataStatus: DataStatus | undefined,
  search: ModelCatalogSearch
): ModelCatalogEmptyState {
  if (hasContentFilter(search)) {
    return {
      descriptionKey: 'modelCatalog.empty.filteredDescription',
      titleKey: 'modelCatalog.empty.filteredTitle',
    }
  }

  if (dataStatus === 'complete') {
    return {
      descriptionKey: 'modelCatalog.empty.upstreamDescription',
      titleKey: 'modelCatalog.empty.upstreamTitle',
    }
  }

  if (dataStatus === 'unavailable') {
    return {
      descriptionKey: 'modelCatalog.empty.unavailableDescription',
      titleKey: 'modelCatalog.empty.unavailableTitle',
    }
  }

  if (dataStatus === 'partial') {
    return {
      descriptionKey: 'modelCatalog.empty.partialDescription',
      titleKey: 'modelCatalog.empty.partialTitle',
    }
  }

  return {
    descriptionKey: 'modelCatalog.empty.pendingDescription',
    titleKey: 'modelCatalog.empty.pendingTitle',
  }
}

export function getMissingModelEmptyState(
  dataStatus: DataStatus | undefined,
  search: ModelCatalogSearch
): ModelCatalogEmptyState {
  if (search.keyword !== '' || search.siteIds.length > 0) {
    return {
      descriptionKey: 'modelCatalog.missingEmpty.filteredDescription',
      titleKey: 'modelCatalog.missingEmpty.filteredTitle',
    }
  }
  if (dataStatus === 'complete') {
    return {
      descriptionKey: 'modelCatalog.missingEmpty.completeDescription',
      titleKey: 'modelCatalog.missingEmpty.completeTitle',
    }
  }
  if (dataStatus === 'partial') {
    return {
      descriptionKey: 'modelCatalog.missingEmpty.partialDescription',
      titleKey: 'modelCatalog.missingEmpty.partialTitle',
    }
  }
  if (dataStatus === 'unavailable') {
    return {
      descriptionKey: 'modelCatalog.missingEmpty.unavailableDescription',
      titleKey: 'modelCatalog.missingEmpty.unavailableTitle',
    }
  }
  return {
    descriptionKey: 'modelCatalog.missingEmpty.pendingDescription',
    titleKey: 'modelCatalog.missingEmpty.pendingTitle',
  }
}
