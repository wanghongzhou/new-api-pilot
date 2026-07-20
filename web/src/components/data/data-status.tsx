import {
  Alert02Icon,
  CancelCircleIcon,
  CheckmarkCircle02Icon,
  HelpCircleIcon,
  Loading03Icon,
  PauseIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import type { DataStatus } from '@/lib/api-types'

const statusConfig = {
  backfilling: { icon: Loading03Icon, variant: 'primary' },
  complete: { icon: CheckmarkCircle02Icon, variant: 'success' },
  disabled: { icon: PauseIcon, variant: 'neutral' },
  missing: { icon: CancelCircleIcon, variant: 'destructive' },
  partial: { icon: Alert02Icon, variant: 'warning' },
  paused: { icon: PauseIcon, variant: 'neutral' },
  pending: { icon: Loading03Icon, variant: 'primary' },
  unavailable: { icon: HelpCircleIcon, variant: 'neutral' },
} as const

export function DataStatusBadge({ status }: { status: DataStatus }) {
  const { t } = useTranslation()
  const config = statusConfig[status]
  return (
    <Badge variant={config.variant}>
      <HugeiconsIcon icon={config.icon} size={14} strokeWidth={2} />
      {t(dynamicI18nKey('data', `data.${status}`))}
    </Badge>
  )
}
