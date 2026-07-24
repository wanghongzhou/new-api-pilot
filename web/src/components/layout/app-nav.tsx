import { HugeiconsIcon } from '@hugeicons/react'
import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import { navGroups } from './app-nav-config'

export function AppNav() {
  const { t } = useTranslation()
  const { setOpenMobile } = useSidebar()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })

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

              return (
                <SidebarMenuItem key={item.to}>
                  <SidebarMenuButton
                    isActive={active}
                    render={
                      <Link
                        aria-current={active ? 'page' : undefined}
                        onClick={() => setOpenMobile(false)}
                        to={item.to}
                      />
                    }
                    tooltip={label}
                  >
                    <HugeiconsIcon
                      className='shrink-0'
                      icon={item.icon}
                      strokeWidth={2}
                    />
                    <span className='min-w-0 flex-1 truncate'>{label}</span>
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
