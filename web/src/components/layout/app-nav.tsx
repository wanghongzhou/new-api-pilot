import {
  Alert02Icon,
  Chart01Icon,
  DashboardSquare01Icon,
  FileExportIcon,
  ServerStack01Icon,
  Settings02Icon,
  UserAccountIcon,
  UserGroup02Icon,
  UserGroupIcon,
  ViewIcon,
} from '@hugeicons/core-free-icons'
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

type NavItem = {
  icon: typeof DashboardSquare01Icon
  label: string
  to:
    | '/accounts'
    | '/alerts'
    | '/channel-inventory'
    | '/customers'
    | '/dashboard'
    | '/exports'
    | '/financial-operations'
    | '/logs'
    | '/model-catalog'
    | '/performance-history'
    | '/pricing-groups'
    | '/rankings'
    | '/subscription-plans'
    | '/system-tasks'
    | '/sites'
    | '/statistics/global'
    | '/user-inventory'
    | '/upstream-tasks'
    | '/settings/system'
    | '/settings/users'
}

const navGroups: ReadonlyArray<{
  label: string
  items: ReadonlyArray<NavItem>
}> = [
  {
    label: 'Overview',
    items: [
      { icon: DashboardSquare01Icon, label: 'Dashboard', to: '/dashboard' },
      { icon: Alert02Icon, label: 'Alerts', to: '/alerts' },
    ],
  },
  {
    label: 'Operations',
    items: [
      { icon: ServerStack01Icon, label: 'Sites', to: '/sites' },
      { icon: UserGroup02Icon, label: 'Customers', to: '/customers' },
      { icon: UserAccountIcon, label: 'Accounts', to: '/accounts' },
      {
        icon: UserGroupIcon,
        label: 'Upstream user inventory',
        to: '/user-inventory',
      },
      {
        icon: ServerStack01Icon,
        label: 'Channel inventory',
        to: '/channel-inventory',
      },
      {
        icon: FileExportIcon,
        label: 'Financial operations',
        to: '/financial-operations',
      },
      { icon: FileExportIcon, label: 'Upstream tasks', to: '/upstream-tasks' },
      { icon: FileExportIcon, label: 'System tasks', to: '/system-tasks' },
    ],
  },
  {
    label: 'Catalog',
    items: [
      { icon: ServerStack01Icon, label: 'Model catalog', to: '/model-catalog' },
      {
        icon: ServerStack01Icon,
        label: 'Pricing and groups',
        to: '/pricing-groups',
      },
      {
        icon: ServerStack01Icon,
        label: 'Subscription plans',
        to: '/subscription-plans',
      },
    ],
  },
  {
    label: 'Analytics',
    items: [
      { icon: Chart01Icon, label: 'Statistics', to: '/statistics/global' },
      { icon: Chart01Icon, label: 'Rankings', to: '/rankings' },
      {
        icon: Chart01Icon,
        label: 'Performance history',
        to: '/performance-history',
      },
      { icon: ViewIcon, label: 'Logs', to: '/logs' },
      { icon: FileExportIcon, label: 'Exports', to: '/exports' },
    ],
  },
  {
    label: 'Settings and access',
    items: [
      {
        icon: Settings02Icon,
        label: 'System settings',
        to: '/settings/system',
      },
      { icon: UserGroupIcon, label: 'Platform users', to: '/settings/users' },
    ],
  },
]

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
