import {
  Alert02Icon,
  CancelCircleIcon,
  CheckmarkCircle02Icon,
  HelpCircleIcon,
  Key01Icon,
  Loading03Icon,
  PauseIcon,
  WifiConnected01Icon,
  WifiDisconnected01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import type { Badge as BadgeComponent } from '@/components/ui/badge'
import type { SiteListItem } from '@/features/sites/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'

type BadgeVariant = NonNullable<Parameters<typeof BadgeComponent>[0]['variant']>

interface StatusItem {
  icon: typeof CheckmarkCircle02Icon
  key: string
  variant: BadgeVariant
}

function siteStatusItems(site: SiteListItem): StatusItem[] {
  const management: StatusItem =
    site.management_status === 'active'
      ? {
          icon: CheckmarkCircle02Icon,
          key: 'site.management.active',
          variant: 'success',
        }
      : {
          icon: PauseIcon,
          key: 'site.management.disabled',
          variant: 'neutral',
        }

  let online: StatusItem
  switch (site.online_status) {
    case 'online':
      online = {
        icon: WifiConnected01Icon,
        key: 'site.online.online',
        variant: 'success',
      }
      break
    case 'offline':
      online = {
        icon: WifiDisconnected01Icon,
        key: 'site.online.offline',
        variant: 'destructive',
      }
      break
    case 'unknown':
      online = {
        icon: HelpCircleIcon,
        key: 'site.online.unknown',
        variant: 'neutral',
      }
      break
  }

  let authorization: StatusItem
  switch (site.auth_status) {
    case 'authorized':
      authorization = {
        icon: Key01Icon,
        key: 'site.auth.authorized',
        variant: 'success',
      }
      break
    case 'expired':
      authorization = {
        icon: Alert02Icon,
        key: 'site.auth.expired',
        variant: 'destructive',
      }
      break
    case 'unauthorized':
      authorization = {
        icon: CancelCircleIcon,
        key: 'site.auth.unauthorized',
        variant: 'warning',
      }
      break
  }

  let statisticsIcon = Alert02Icon
  let statisticsVariant: BadgeVariant = 'destructive'
  switch (site.statistics_status) {
    case 'ready':
      statisticsIcon = CheckmarkCircle02Icon
      statisticsVariant = 'success'
      break
    case 'backfilling':
      statisticsIcon = Loading03Icon
      statisticsVariant = 'primary'
      break
    case 'partial':
    case 'pending_config':
      statisticsVariant = 'warning'
      break
    case 'paused':
      statisticsIcon = PauseIcon
      statisticsVariant = 'neutral'
      break
    case 'error':
      break
  }

  let healthIcon = Alert02Icon
  let healthVariant: BadgeVariant = 'neutral'
  switch (site.health_status) {
    case 'ok':
      healthIcon = CheckmarkCircle02Icon
      healthVariant = 'success'
      break
    case 'warning':
      healthVariant = 'warning'
      break
    case 'critical':
      healthVariant = 'destructive'
      break
    case 'unavailable':
      healthIcon = HelpCircleIcon
      break
  }

  return [
    management,
    online,
    authorization,
    {
      icon: statisticsIcon,
      key: `site.statistics.${site.statistics_status}`,
      variant: statisticsVariant,
    },
    {
      icon: healthIcon,
      key: `site.health.${site.health_status}`,
      variant: healthVariant,
    },
  ]
}

export function SiteStatusBadges({ site }: { site: SiteListItem }) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap gap-1.5'>
      {siteStatusItems(site).map((item) => (
        <Badge key={item.key} variant={item.variant}>
          <HugeiconsIcon icon={item.icon} size={14} strokeWidth={2} />
          {t(dynamicI18nKey('data', item.key))}
        </Badge>
      ))}
    </div>
  )
}
