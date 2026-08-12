import type { DashboardSiteHealthItem } from './types'

export function isDashboardProblemSite(site: DashboardSiteHealthItem) {
  return (
    site.management_status !== 'active' ||
    site.online_status !== 'online' ||
    site.auth_status !== 'authorized' ||
    site.statistics_status !== 'ready' ||
    site.health_status !== 'ok'
  )
}
