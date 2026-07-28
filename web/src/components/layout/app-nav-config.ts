import {
  AiSearchIcon,
  Alert02Icon,
  Analytics01Icon,
  Audit02Icon,
  Building03Icon,
  DashboardSquare01Icon,
  FileExportIcon,
  LaptopPerformanceIcon,
  Money03Icon,
  MoneyBag02Icon,
  Package02Icon,
  RankingIcon,
  ServerStack01Icon,
  Settings02Icon,
  SystemUpdate01Icon,
  Task02Icon,
  UserAccountIcon,
  UserGroup02Icon,
  UserListIcon,
  UserShield01Icon,
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
    label: 'Workspace',
    items: [
      {
        icon: DashboardSquare01Icon,
        label: 'Operations overview',
        to: '/dashboard',
      },
      { icon: Alert02Icon, label: 'Alerts', to: '/alerts' },
    ],
  },
  {
    label: 'Business management',
    items: [
      { icon: Building03Icon, label: 'Sites', to: '/sites' },
      { icon: UserGroup02Icon, label: 'Customers', to: '/customers' },
      { icon: UserAccountIcon, label: 'Accounts', to: '/accounts' },
    ],
  },
  {
    label: 'Tasks and logs',
    items: [
      { icon: Audit02Icon, label: 'Usage logs', to: '/logs' },
      { icon: Task02Icon, label: 'Task logs', to: '/upstream-tasks' },
      { icon: SystemUpdate01Icon, label: 'System tasks', to: '/system-tasks' },
      { icon: FileExportIcon, label: 'Export center', to: '/exports' },
    ],
  },
  {
    label: 'Operations analytics',
    items: [
      {
        icon: Money03Icon,
        label: 'Financial operations',
        to: '/financial-operations',
      },
      {
        icon: Analytics01Icon,
        label: 'Global statistics',
        to: '/statistics/global',
      },
      { icon: RankingIcon, label: 'Rankings', to: '/rankings' },
      {
        icon: LaptopPerformanceIcon,
        label: 'Performance trends',
        to: '/performance-history',
      },
    ],
  },
  {
    label: 'Resource center',
    items: [
      {
        icon: UserListIcon,
        label: 'Upstream user inventory',
        to: '/user-inventory',
      },
      {
        icon: ServerStack01Icon,
        label: 'Channel inventory',
        to: '/channel-inventory',
      },
      { icon: AiSearchIcon, label: 'Model catalog', to: '/model-catalog' },
      {
        icon: MoneyBag02Icon,
        label: 'Pricing and groups',
        to: '/pricing-groups',
      },
      {
        icon: Package02Icon,
        label: 'Subscription plans',
        to: '/subscription-plans',
      },
    ],
  },
  {
    label: 'Platform administration',
    items: [
      {
        icon: UserShield01Icon,
        label: 'Platform users',
        to: '/settings/users',
      },
      {
        icon: Settings02Icon,
        label: 'System settings',
        to: '/settings/system',
      },
    ],
  },
]
