import { isIdString, parseIdString } from '@/lib/api-types'

import type { RankingPeriod, RankingTab } from './types'

export type RankingView = 'history' | 'movement' | 'ranking' | 'sites'

export interface RankingSearch {
  page: number
  pageSize: number
  period: RankingPeriod
  tab: RankingTab
  view: RankingView
  siteIds: ReturnType<typeof parseIdString>[]
  exportId?: ReturnType<typeof parseIdString>
}

export function buildRankingSearch(raw: {
  exportId?: string
  page?: number
  pageSize?: number
  period?: string
  siteIds?: readonly string[]
  tab?: string
  view?: string
}): RankingSearch {
  const page =
    typeof raw.page === 'number' && Number.isInteger(raw.page) && raw.page > 0
      ? raw.page
      : 1
  return {
    exportId:
      typeof raw.exportId === 'string' && isIdString(raw.exportId)
        ? parseIdString(raw.exportId)
        : undefined,
    page,
    pageSize:
      raw.pageSize === 10 ||
      raw.pageSize === 20 ||
      raw.pageSize === 50 ||
      raw.pageSize === 100
        ? raw.pageSize
        : 20,
    period:
      raw.period === 'today' ||
      raw.period === 'week' ||
      raw.period === 'month' ||
      raw.period === 'year'
        ? raw.period
        : 'month',
    siteIds: [...new Set(raw.siteIds ?? [])]
      .filter(isIdString)
      .map(parseIdString)
      .sort((left, right) => left.localeCompare(right)),
    tab: raw.tab === 'vendors' ? 'vendors' : 'models',
    view:
      raw.view === 'movement' || raw.view === 'history' || raw.view === 'sites'
        ? raw.view
        : 'ranking',
  }
}
