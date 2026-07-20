import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  LocalRankingResponse,
  RankingQueryParams,
  RankingTab,
} from './types'

function params(values: RankingQueryParams) {
  const params = new URLSearchParams()
  params.set('period', values.period)
  for (const siteId of values.site_ids ?? []) params.append('site_ids', siteId)
  return params
}

function path(tab: RankingTab) {
  return tab === 'models' ? 'models' : 'vendors'
}

export function getRankings(tab: RankingTab, values: RankingQueryParams) {
  return requestApiData<LocalRankingResponse>({
    method: 'get',
    params: params(values),
    url: `/api/rankings/${path(tab)}`,
  })
}

export function getSiteRankings(
  siteId: IdString,
  tab: RankingTab,
  values: RankingQueryParams
) {
  return requestApiData<LocalRankingResponse>({
    method: 'get',
    params: params({ ...values, site_ids: undefined }),
    url: `/api/sites/${siteId}/rankings/${path(tab)}`,
  })
}
