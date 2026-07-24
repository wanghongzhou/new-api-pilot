import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  MissingModelPage,
  ModelCatalogPage,
  ModelCatalogQueryParams,
  ModelCoverageResponse,
} from './types'

function appendValues(
  params: URLSearchParams,
  key: string,
  values: readonly (number | string)[] | undefined
) {
  for (const value of values ?? []) params.append(key, String(value))
}

function catalogParams(
  values: ModelCatalogQueryParams,
  suffix: '' | '/coverage' | '/missing'
) {
  const params = new URLSearchParams()
  params.set('p', String(values.p))
  params.set('page_size', String(values.page_size))
  appendValues(params, 'site_ids', values.site_ids)
  if (suffix !== '/missing') {
    appendValues(params, 'statuses', values.statuses)
    appendValues(params, 'sync_official', values.sync_official)
    if (values.vendor_id != null) params.set('vendor_id', values.vendor_id)
  }
  if (values.keyword) params.set('keyword', values.keyword)
  return params
}

function requestCatalog<T>(
  suffix: '' | '/coverage' | '/missing',
  values: ModelCatalogQueryParams,
  siteId?: IdString
) {
  return requestApiData<T>({
    method: 'get',
    params: catalogParams(
      {
        ...values,
        site_ids: siteId ? undefined : values.site_ids,
      },
      suffix
    ),
    url: siteId
      ? `/api/sites/${siteId}/model-catalog${suffix}`
      : `/api/model-catalog${suffix}`,
  })
}

export const listModelCatalog = (values: ModelCatalogQueryParams) =>
  requestCatalog<ModelCatalogPage>('', values)
export const listSiteModelCatalog = (
  siteId: IdString,
  values: ModelCatalogQueryParams
) => requestCatalog<ModelCatalogPage>('', values, siteId)
export const getModelCoverage = (values: ModelCatalogQueryParams) =>
  requestCatalog<ModelCoverageResponse>('/coverage', values)
export const getSiteModelCoverage = (
  siteId: IdString,
  values: ModelCatalogQueryParams
) => requestCatalog<ModelCoverageResponse>('/coverage', values, siteId)
export const listMissingModels = (values: ModelCatalogQueryParams) =>
  requestCatalog<MissingModelPage>('/missing', values)
export const listSiteMissingModels = (
  siteId: IdString,
  values: ModelCatalogQueryParams
) => requestCatalog<MissingModelPage>('/missing', values, siteId)
