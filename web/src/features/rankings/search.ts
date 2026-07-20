import { isIdString, parseIdString } from '@/lib/api-types'

import type { RankingPeriod, RankingTab } from './types'

export interface RankingSearch {
  period: RankingPeriod
  tab: RankingTab
  siteIds: ReturnType<typeof parseIdString>[]
  exportId?: ReturnType<typeof parseIdString>
}

export function buildRankingSearch(raw: {
  exportId?: string
  period?: string
  siteIds?: readonly string[]
  tab?: string
}): RankingSearch {
  return {
    exportId:
      typeof raw.exportId === 'string' && isIdString(raw.exportId)
        ? parseIdString(raw.exportId)
        : undefined,
    period:
      raw.period === 'week' || raw.period === 'month' || raw.period === 'year'
        ? raw.period
        : 'today',
    siteIds: [...new Set(raw.siteIds ?? [])]
      .filter(isIdString)
      .map(parseIdString)
      .sort((left, right) => left.localeCompare(right)),
    tab: raw.tab === 'vendors' ? 'vendors' : 'models',
  }
}
