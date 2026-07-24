import type {
  EntityStatisticsParams,
  SiteStatisticsBreakdown,
  StatisticsResponse,
} from '@/features/statistics/types'
import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  CollectionRunItem,
  CollectionRunListParams,
  CollectionRunPage,
  CollectionRunWindowListParams,
  CollectionRunWindowPage,
  FastTaskHistoryListParams,
  FastTaskHistoryPage,
  SiteAuthorizationResult,
  SiteAuthorizeRequest,
  SiteBackfillRequest,
  SiteBaseUrlPreflightResult,
  SiteCreateRequest,
  SiteDetail,
  SiteInstanceItem,
  SiteListParams,
  SitePage,
  SitePerformanceSummary,
  SiteProbeResult,
  SiteResourceQuery,
  SiteResourceResponse,
  SiteUpdateRequest,
} from './types'

type SearchValue = string | number | readonly string[] | undefined

function toSearchParams<T extends object>(values: T): URLSearchParams {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(values) as [
    string,
    SearchValue,
  ][]) {
    if (value === undefined || value === '') continue
    if (Array.isArray(value)) {
      for (const item of value) params.append(key, item)
    } else {
      params.set(key, String(value))
    }
  }
  return params
}

export function listSites(params: SiteListParams): Promise<SitePage> {
  const searchParams = toSearchParams(params)
  return requestApiData<SitePage>({
    method: 'get',
    params: searchParams.size > 0 ? searchParams : undefined,
    url: '/api/sites',
  })
}

export function createSite(request: SiteCreateRequest): Promise<SiteDetail> {
  return requestApiData<SiteDetail>({
    data: request,
    method: 'post',
    url: '/api/sites',
  })
}

export function getSite(id: IdString): Promise<SiteDetail> {
  return requestApiData<SiteDetail>({ method: 'get', url: `/api/sites/${id}` })
}

export function getSitePerformance(
  id: IdString,
  hours: number
): Promise<SitePerformanceSummary> {
  return requestApiData<SitePerformanceSummary>({
    method: 'get',
    params: { hours },
    url: `/api/sites/${id}/performance`,
  })
}

export function getSiteStatistics(
  id: IdString,
  params: EntityStatisticsParams
): Promise<StatisticsResponse<SiteStatisticsBreakdown>> {
  return requestApiData<StatisticsResponse<SiteStatisticsBreakdown>>({
    method: 'get',
    params: toSearchParams(params),
    url: `/api/sites/${id}/stats`,
  })
}

export function preflightSiteBaseUrl(
  id: IdString,
  baseUrl: string
): Promise<SiteBaseUrlPreflightResult> {
  return requestApiData<SiteBaseUrlPreflightResult>({
    data: { base_url: baseUrl },
    method: 'post',
    url: `/api/sites/${id}/base-url-preflight`,
  })
}

export function updateSite(
  id: IdString,
  request: SiteUpdateRequest
): Promise<SiteDetail> {
  return requestApiData<SiteDetail>({
    data: request,
    method: 'put',
    url: `/api/sites/${id}`,
  })
}

export function deleteSite(id: IdString): Promise<null> {
  return requestApiData<null>({
    method: 'delete',
    url: `/api/sites/${id}`,
  })
}

export function authorizeSite(
  id: IdString,
  request: SiteAuthorizeRequest
): Promise<SiteAuthorizationResult> {
  return requestApiData<SiteAuthorizationResult>({
    data: request,
    method: 'post',
    url: `/api/sites/${id}/authorize`,
  })
}

export function recheckSiteCapabilities(
  id: IdString
): Promise<SiteAuthorizationResult> {
  return requestApiData<SiteAuthorizationResult>({
    method: 'post',
    url: `/api/sites/${id}/recheck-capabilities`,
  })
}

export function probeSite(id: IdString): Promise<SiteProbeResult> {
  return requestApiData<SiteProbeResult>({
    method: 'post',
    url: `/api/sites/${id}/probe`,
  })
}

export function refreshSites(
  siteIds: IdString[]
): Promise<CollectionRunItem[]> {
  return requestApiData<CollectionRunItem[]>({
    data: { site_ids: siteIds },
    method: 'post',
    url: '/api/sites/refresh',
  })
}

export function refreshSite(id: IdString): Promise<CollectionRunItem[]> {
  return requestApiData<CollectionRunItem[]>({
    method: 'post',
    url: `/api/sites/${id}/refresh`,
  })
}

export function backfillSite(
  id: IdString,
  request: SiteBackfillRequest
): Promise<CollectionRunItem> {
  return requestApiData<CollectionRunItem>({
    data: request,
    method: 'post',
    url: `/api/sites/${id}/backfill`,
  })
}

export function disableSite(id: IdString): Promise<SiteDetail> {
  return requestApiData<SiteDetail>({
    method: 'post',
    url: `/api/sites/${id}/disable`,
  })
}

export function enableSite(id: IdString): Promise<CollectionRunItem> {
  return requestApiData<CollectionRunItem>({
    method: 'post',
    url: `/api/sites/${id}/enable`,
  })
}

export function endSiteStatistics(
  id: IdString,
  statisticsEndAt: number
): Promise<SiteDetail> {
  return requestApiData<SiteDetail>({
    data: { statistics_end_at: statisticsEndAt },
    method: 'post',
    url: `/api/sites/${id}/end-statistics`,
  })
}

export function clearSiteStatisticsEnd(id: IdString): Promise<SiteDetail> {
  return requestApiData<SiteDetail>({
    method: 'delete',
    url: `/api/sites/${id}/statistics-end`,
  })
}

export function listSiteInstances(id: IdString): Promise<SiteInstanceItem[]> {
  return requestApiData<SiteInstanceItem[]>({
    method: 'get',
    url: `/api/sites/${id}/instances`,
  })
}

export function getSiteResource(
  id: IdString,
  query: SiteResourceQuery
): Promise<SiteResourceResponse> {
  return requestApiData<SiteResourceResponse>({
    method: 'get',
    params: query,
    url: `/api/sites/${id}/status`,
  })
}

export function listSiteCollectionRuns(
  id: IdString,
  params: CollectionRunListParams
): Promise<CollectionRunPage> {
  return requestApiData<CollectionRunPage>({
    method: 'get',
    params: toSearchParams(params),
    url: `/api/sites/${id}/collection-runs`,
  })
}

export function listSiteFastTaskHistory(
  params: FastTaskHistoryListParams
): Promise<FastTaskHistoryPage> {
  return requestApiData<FastTaskHistoryPage>({
    method: 'get',
    params: toSearchParams(params),
    url: '/api/fast-tasks',
  })
}

export function getCollectionRun(id: IdString): Promise<CollectionRunItem> {
  return requestApiData<CollectionRunItem>({
    method: 'get',
    url: `/api/collection-runs/${id}`,
  })
}

export function listCollectionRunWindows(
  id: IdString,
  params: CollectionRunWindowListParams
): Promise<CollectionRunWindowPage> {
  return requestApiData<CollectionRunWindowPage>({
    method: 'get',
    params: toSearchParams(params),
    url: `/api/collection-runs/${id}/windows`,
  })
}
