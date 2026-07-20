import { requestApiData } from '@/lib/api'

import type {
  DashboardHealth,
  DashboardRankingItem,
  DashboardSummary,
  DashboardTopMetric,
  DashboardTopType,
  DashboardTrend,
} from './types'

export function getDashboardSummary(): Promise<DashboardSummary> {
  return requestApiData<DashboardSummary>({
    method: 'get',
    url: '/api/dashboard/summary',
  })
}

export function getDashboardTrend(days = 30): Promise<DashboardTrend> {
  return requestApiData<DashboardTrend>({
    method: 'get',
    params: { days },
    url: '/api/dashboard/trend',
  })
}

export function getDashboardTop(
  type: DashboardTopType,
  metric: DashboardTopMetric,
  limit = 5
): Promise<DashboardRankingItem[]> {
  if (!Number.isInteger(limit) || limit < 1 || limit > 20) {
    throw new RangeError(
      'Dashboard ranking limit must be an integer from 1 to 20'
    )
  }
  return requestApiData<DashboardRankingItem[]>({
    method: 'get',
    params: { limit, metric, type },
    url: '/api/dashboard/top',
  })
}

export function getDashboardHealth(): Promise<DashboardHealth> {
  return requestApiData<DashboardHealth>({
    method: 'get',
    url: '/api/dashboard/health',
  })
}
