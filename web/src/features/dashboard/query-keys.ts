import type { DashboardTopMetric, DashboardTopType } from './types'

export const dashboardKeys = {
  all: ['dashboard'] as const,
  summary: () => ['dashboard', 'summary'] as const,
  trend: (days: number) => ['dashboard', 'trend', { days }] as const,
  top: (type: DashboardTopType, metric: DashboardTopMetric, limit: number) =>
    ['dashboard', 'top', { limit, metric, type }] as const,
  health: () => ['dashboard', 'health'] as const,
}
