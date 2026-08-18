import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useRouterState } from '@tanstack/react-router'
import type { MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar'
import { getAlertSummary } from '@/features/alerts/api'
import { alertKeys } from '@/features/alerts/query-keys'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import { resolveAlertNavBadge } from './app-nav-badge'
import { navGroups } from './app-nav-config'

export const APP_NAVIGATE_EVENT = 'pilot:navigate'

export function AppNav() {
  const { t } = useTranslation()
  const { setOpenMobile } = useSidebar()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const alertSummaryQuery = useQuery({
    queryFn: getAlertSummary,
    queryKey: alertKeys.summary(),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })
  const firingAlertCount = alertSummaryQuery.data?.firing_count
  const alertBadge = resolveAlertNavBadge(
    firingAlertCount,
    alertSummaryQuery.isError
  )

  return (
    <nav aria-label={t('Primary navigation')}>
      {navGroups.map((group) => (
        <SidebarGroup className='px-2 py-1' key={group.label}>
          <SidebarGroupLabel className='text-muted-foreground/70 px-2 text-[11px] font-medium tracking-wider uppercase'>
            {t(dynamicI18nKey('layout', group.label))}
          </SidebarGroupLabel>
          <SidebarMenu>
            {group.items.map((item) => {
              const active =
                (item.to === '/statistics/global' &&
                  pathname.startsWith('/statistics/')) ||
                pathname === item.to ||
                pathname.startsWith(`${item.to}/`)
              const label = t(dynamicI18nKey('layout', item.label))
              const handleNavigationClick = (
                event: MouseEvent<HTMLAnchorElement>
              ) => {
                const navigationEvent = new CustomEvent(APP_NAVIGATE_EVENT, {
                  cancelable: true,
                  detail: { to: item.to },
                })
                if (!window.dispatchEvent(navigationEvent)) {
                  event.preventDefault()
                  return
                }
                setOpenMobile(false)
              }
              let alertBadgeLabel = t('alerts.navigation.firingCount', {
                count: firingAlertCount ?? 0,
              })
              if (alertBadge.kind === 'unknown') {
                alertBadgeLabel = t('alerts.navigation.countUnknown')
              } else if (alertBadge.kind === 'stale') {
                alertBadgeLabel = t('alerts.navigation.countStale', {
                  count: firingAlertCount ?? 0,
                })
              }

              return (
                <SidebarMenuItem key={item.to}>
                  <SidebarMenuButton
                    isActive={active}
                    render={
                      <a
                        aria-current={active ? 'page' : undefined}
                        href={item.to}
                        onClick={handleNavigationClick}
                      />
                    }
                  >
                    <HugeiconsIcon
                      className='shrink-0'
                      icon={item.icon}
                      strokeWidth={2}
                    />
                    <span className='min-w-0 flex-1 truncate'>{label}</span>
                    {item.to === '/alerts' && alertBadge.text ? (
                      <Badge
                        aria-label={alertBadgeLabel}
                        className='ml-auto h-5 min-w-5 px-1.5 tabular-nums group-data-[collapsible=icon]:hidden'
                        title={alertBadgeLabel}
                        variant={
                          alertBadge.kind === 'count'
                            ? 'destructive'
                            : 'warning'
                        }
                      >
                        {alertBadge.text}
                      </Badge>
                    ) : null}
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )
            })}
          </SidebarMenu>
        </SidebarGroup>
      ))}
    </nav>
  )
}
