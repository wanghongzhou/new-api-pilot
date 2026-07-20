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

import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { cn } from '@/lib/utils'

const navItems: ReadonlyArray<{
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
}> = [
  { icon: DashboardSquare01Icon, label: 'Dashboard', to: '/dashboard' },
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
    icon: Chart01Icon,
    label: 'Performance history',
    to: '/performance-history',
  },
  {
    icon: FileExportIcon,
    label: 'Financial operations',
    to: '/financial-operations',
  },
  {
    icon: FileExportIcon,
    label: 'Upstream tasks',
    to: '/upstream-tasks',
  },
  {
    icon: ServerStack01Icon,
    label: 'Model catalog',
    to: '/model-catalog',
  },
  {
    icon: ServerStack01Icon,
    label: 'Pricing and groups',
    to: '/pricing-groups',
  },
  { icon: Chart01Icon, label: 'Statistics', to: '/statistics/global' },
  { icon: Chart01Icon, label: 'Rankings', to: '/rankings' },
  {
    icon: ServerStack01Icon,
    label: 'Subscription plans',
    to: '/subscription-plans',
  },
  {
    icon: FileExportIcon,
    label: 'System tasks',
    to: '/system-tasks',
  },
  { icon: ViewIcon, label: 'Logs', to: '/logs' },
  { icon: FileExportIcon, label: 'Exports', to: '/exports' },
  { icon: Alert02Icon, label: 'Alerts', to: '/alerts' },
  { icon: Settings02Icon, label: 'System settings', to: '/settings/system' },
  { icon: UserGroupIcon, label: 'Platform users', to: '/settings/users' },
]

export function AppNav({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useTranslation()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })

  return (
    <nav aria-label={t('Primary navigation')} className='grid gap-1'>
      {navItems.map((item) => {
        const active =
          (item.to === '/statistics/global' &&
            pathname.startsWith('/statistics/')) ||
          pathname === item.to ||
          pathname.startsWith(`${item.to}/`)
        return (
          <Link
            aria-current={active ? 'page' : undefined}
            className={cn(
              'text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground flex min-h-10 items-center gap-3 rounded-md px-3 text-sm font-medium transition-colors',
              active && 'bg-sidebar-accent text-sidebar-accent-foreground'
            )}
            key={item.to}
            onClick={onNavigate}
            to={item.to}
          >
            <HugeiconsIcon icon={item.icon} strokeWidth={2} />
            <span>{t(dynamicI18nKey('layout', item.label))}</span>
          </Link>
        )
      })}
    </nav>
  )
}
