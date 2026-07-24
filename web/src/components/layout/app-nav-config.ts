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
    | '/settings/system'
    | '/settings/users'
    | '/sites'
    | '/statistics/global'
    | '/subscription-plans'
    | '/system-tasks'
    | '/upstream-tasks'
    | '/user-inventory'
}

export const navGroups: ReadonlyArray<{
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
      { icon: UserGroupIcon, label: 'Platform users', to: '/settings/users' },
      {
        icon: Settings02Icon,
        label: 'System settings',
        to: '/settings/system',
      },
    ],
  },
]
