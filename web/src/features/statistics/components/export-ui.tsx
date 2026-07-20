import {
  CancelCircleIcon,
  CheckmarkCircle02Icon,
  Clock01Icon,
  Loading03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { fromUnixSeconds } from '@/lib/dayjs'

import type {
  StatisticsExportFormat,
  StatisticsExportScope,
  StatisticsExportStatus,
} from '../types'

export function exportStatusText(
  t: (key: string) => string,
  status: StatisticsExportStatus
): string {
  switch (status) {
    case 'pending':
      return t('statistics.export.status.pending')
    case 'running':
      return t('statistics.export.status.running')
    case 'success':
      return t('statistics.export.status.success')
    case 'failed':
      return t('statistics.export.status.failed')
    case 'expired':
      return t('statistics.export.status.expired')
  }
}

export function exportScopeText(
  t: (key: string) => string,
  scope: StatisticsExportScope
): string {
  switch (scope) {
    case 'global':
      return t('statistics.scope.global')
    case 'site':
      return t('statistics.scope.site')
    case 'customer':
      return t('statistics.scope.customer')
    case 'account':
      return t('statistics.scope.account')
    case 'model':
      return t('statistics.scope.model')
    case 'channel':
      return t('statistics.scope.channel')
    case 'group':
      return t('statistics.scope.group')
    case 'token':
      return t('statistics.scope.token')
    case 'node':
      return t('statistics.scope.node')
    case 'logs':
      return t('logs.title')
    case 'user_inventory':
      return t('userInventory.title')
    case 'channel_inventory':
      return t('statistics.scope.channel')
    case 'performance_history':
      return t('site.performance.title')
    case 'topup_inventory':
      return t('financialOperations.tabs.topups')
    case 'redemption_inventory':
      return t('financialOperations.tabs.redemptions')
    case 'upstream_tasks':
      return t('upstreamTasks.title')
    case 'model_catalog':
      return t('modelCatalog.title')
    case 'model_rankings':
      return t('rankings.tabs.models')
    case 'vendor_rankings':
      return t('rankings.tabs.vendors')
    case 'subscription_plans':
      return t('subscriptionPlans.title')
    case 'pricing_catalog':
      return t('pricingGroups.tabs.pricing')
    case 'group_catalog':
      return t('pricingGroups.tabs.groups')
    case 'system_tasks':
      return t('systemTasks.title')
  }
}

export function exportFormatText(
  t: (key: string) => string,
  format: StatisticsExportFormat
): string {
  return format === 'csv'
    ? t('statistics.export.format.csv')
    : t('statistics.export.format.xlsx')
}

export function ExportStatusBadge({
  status,
}: {
  status: StatisticsExportStatus
}) {
  const { t } = useTranslation()
  let icon = Clock01Icon
  let variant: 'destructive' | 'success' | 'warning' = 'warning'
  if (status === 'running') icon = Loading03Icon
  else if (status === 'success') {
    icon = CheckmarkCircle02Icon
    variant = 'success'
  } else if (status === 'failed' || status === 'expired') {
    icon = CancelCircleIcon
    variant = 'destructive'
  }
  return (
    <Badge variant={variant}>
      <HugeiconsIcon icon={icon} size={14} strokeWidth={2} />
      {exportStatusText(t, status)}
    </Badge>
  )
}

export function ExportTimestamp({ value }: { value: number | null }) {
  const { t } = useTranslation()
  if (value == null) return <span>{t('common.none')}</span>
  const formatted = fromUnixSeconds(value).format('YYYY-MM-DD HH:mm:ss')
  return <time dateTime={String(value)}>{formatted}</time>
}
