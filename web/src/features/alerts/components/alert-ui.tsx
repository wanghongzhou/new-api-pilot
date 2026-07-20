import {
  Alert02Icon,
  AlertCircleIcon,
  CancelCircleIcon,
  CheckmarkCircle02Icon,
  Clock01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { fromUnixSeconds } from '@/lib/dayjs'

import type {
  AlertDeliveryEventType,
  AlertDeliveryStatus,
  AlertLevel,
  AlertRuleItem,
  AlertRuleScope,
  AlertStatus,
  AlertTargetType,
} from '../types'

export function alertRuleName(
  t: (key: string, params?: Record<string, unknown>) => string,
  ruleKey: string
): string {
  switch (ruleKey) {
    case 'site_offline':
      return t('alerts.rule.site_offline')
    case 'site_auth_expired':
      return t('alerts.rule.site_auth_expired')
    case 'site_export_disabled':
      return t('alerts.rule.site_export_disabled')
    case 'collection_missing':
      return t('alerts.rule.collection_missing')
    case 'backfill_failed':
      return t('alerts.rule.backfill_failed')
    case 'validation_failed':
      return t('alerts.rule.validation_failed')
    case 'instance_stale':
      return t('alerts.rule.instance_stale')
    case 'instance_offline':
      return t('alerts.rule.instance_offline')
    case 'site_no_instance':
      return t('alerts.rule.site_no_instance')
    case 'cpu_high':
      return t('alerts.rule.cpu_high')
    case 'memory_high':
      return t('alerts.rule.memory_high')
    case 'disk_high':
      return t('alerts.rule.disk_high')
    case 'account_missing':
      return t('alerts.rule.account_missing')
    case 'account_identity_mismatch':
      return t('alerts.rule.account_identity_mismatch')
    case 'account_disabled':
      return t('alerts.rule.account_disabled')
    case 'account_quota_empty':
      return t('alerts.rule.account_quota_empty')
    case 'channel_balance_low':
      return t('alerts.rule.channel_balance_low')
    case 'channel_response_time_high':
      return t('alerts.rule.channel_response_time_high')
    case 'channel_availability_low':
      return t('alerts.rule.channel_availability_low')
    default:
      return t('alerts.rule.unknown', { key: ruleKey })
  }
}

export function alertRuleDescription(
  t: (key: string, params?: Record<string, unknown>) => string,
  ruleKey: string
): string {
  switch (ruleKey) {
    case 'site_offline':
      return t('alerts.ruleDescription.site_offline')
    case 'site_auth_expired':
      return t('alerts.ruleDescription.site_auth_expired')
    case 'site_export_disabled':
      return t('alerts.ruleDescription.site_export_disabled')
    case 'collection_missing':
      return t('alerts.ruleDescription.collection_missing')
    case 'backfill_failed':
      return t('alerts.ruleDescription.backfill_failed')
    case 'validation_failed':
      return t('alerts.ruleDescription.validation_failed')
    case 'instance_stale':
      return t('alerts.ruleDescription.instance_stale')
    case 'instance_offline':
      return t('alerts.ruleDescription.instance_offline')
    case 'site_no_instance':
      return t('alerts.ruleDescription.site_no_instance')
    case 'cpu_high':
      return t('alerts.ruleDescription.cpu_high')
    case 'memory_high':
      return t('alerts.ruleDescription.memory_high')
    case 'disk_high':
      return t('alerts.ruleDescription.disk_high')
    case 'account_missing':
      return t('alerts.ruleDescription.account_missing')
    case 'account_identity_mismatch':
      return t('alerts.ruleDescription.account_identity_mismatch')
    case 'account_disabled':
      return t('alerts.ruleDescription.account_disabled')
    case 'account_quota_empty':
      return t('alerts.ruleDescription.account_quota_empty')
    case 'channel_balance_low':
      return t('alerts.ruleDescription.channel_balance_low')
    case 'channel_response_time_high':
      return t('alerts.ruleDescription.channel_response_time_high')
    case 'channel_availability_low':
      return t('alerts.ruleDescription.channel_availability_low')
    default:
      return t('alerts.ruleDescription.unknown')
  }
}

export function alertTargetTypeText(
  t: (key: string) => string,
  targetType: AlertTargetType
): string {
  switch (targetType) {
    case 'site':
      return t('alerts.target.site')
    case 'instance':
      return t('alerts.target.instance')
    case 'account':
      return t('alerts.target.account')
    case 'collection':
      return t('alerts.target.collection')
  }
}

export function alertLevelText(
  t: (key: string) => string,
  level: AlertLevel
): string {
  switch (level) {
    case 'critical':
      return t('alerts.level.critical')
    case 'warning':
      return t('alerts.level.warning')
    case 'info':
      return t('alerts.level.info')
  }
}

export function alertStatusText(
  t: (key: string) => string,
  status: AlertStatus
): string {
  switch (status) {
    case 'firing':
      return t('alerts.status.firing')
    case 'pending':
      return t('alerts.status.pending')
    case 'resolved':
      return t('alerts.status.resolved')
  }
}

export function AlertLevelBadge({ level }: { level: AlertLevel }) {
  const { t } = useTranslation()
  const icon = level === 'critical' ? AlertCircleIcon : Alert02Icon
  let variant: 'destructive' | 'neutral' | 'warning' = 'neutral'
  if (level === 'critical') variant = 'destructive'
  else if (level === 'warning') variant = 'warning'
  return (
    <Badge variant={variant}>
      <HugeiconsIcon icon={icon} size={14} strokeWidth={2} />
      {alertLevelText(t, level)}
    </Badge>
  )
}

export function AlertStatusBadge({ status }: { status: AlertStatus }) {
  const { t } = useTranslation()
  let icon = Clock01Icon
  let variant: 'destructive' | 'success' | 'warning' = 'warning'
  if (status === 'resolved') {
    icon = CheckmarkCircle02Icon
    variant = 'success'
  } else if (status === 'firing') {
    icon = AlertCircleIcon
    variant = 'destructive'
  }
  return (
    <Badge variant={variant}>
      <HugeiconsIcon icon={icon} size={14} strokeWidth={2} />
      {alertStatusText(t, status)}
    </Badge>
  )
}

export function DeliveryStatusBadge({
  status,
}: {
  status: AlertDeliveryStatus
}) {
  const { t } = useTranslation()
  let label: string
  let icon = Clock01Icon
  let variant: 'destructive' | 'success' | 'warning' = 'warning'
  if (status === 'success') {
    label = t('alerts.delivery.status.success')
    icon = CheckmarkCircle02Icon
    variant = 'success'
  } else if (status === 'failed') {
    label = t('alerts.delivery.status.failed')
    icon = CancelCircleIcon
    variant = 'destructive'
  } else label = t('alerts.delivery.status.pending')
  return (
    <Badge variant={variant}>
      <HugeiconsIcon icon={icon} size={14} strokeWidth={2} />
      {label}
    </Badge>
  )
}

export function deliveryEventTypeText(
  t: (key: string) => string,
  eventType: AlertDeliveryEventType
): string {
  switch (eventType) {
    case 'firing':
      return t('alerts.delivery.event.firing')
    case 'resolved':
      return t('alerts.delivery.event.resolved')
    case 'test':
      return t('alerts.delivery.event.test')
  }
}

export function ruleScopeText(
  t: (key: string) => string,
  scope: AlertRuleScope
): string {
  return scope === 'global'
    ? t('alerts.rules.scope.global')
    : t('alerts.rules.scope.site')
}

export function RuleScopeBadge({ rule }: { rule: AlertRuleItem }) {
  const { t } = useTranslation()
  return (
    <Badge variant={rule.inherited ? 'neutral' : 'primary'}>
      {rule.inherited
        ? t('alerts.rules.inherited')
        : ruleScopeText(t, rule.scope_type)}
    </Badge>
  )
}

export function AlertTime({ timestamp }: { timestamp: number | null }) {
  const { t } = useTranslation()
  if (timestamp == null) return <span>{t('alerts.value.unavailable')}</span>
  const formatted = fromUnixSeconds(timestamp).format('YYYY-MM-DD HH:mm:ss')
  return <time dateTime={String(timestamp)}>{formatted}</time>
}

export function alertDeliveryErrorText(
  t: (key: string, params?: Record<string, unknown>) => string,
  errorCode: string
): string {
  switch (errorCode) {
    case 'NOTIFICATION_DISABLED':
      return t('NOTIFICATION_DISABLED')
    case 'NOTIFICATION_NOT_CONFIGURED':
      return t('NOTIFICATION_NOT_CONFIGURED')
    case 'DINGTALK_ADDRESS_FORBIDDEN':
      return t('DINGTALK_ADDRESS_FORBIDDEN')
    case 'DINGTALK_REJECTED':
      return t('DINGTALK_REJECTED')
    case 'DELIVERY_RETRY_EXHAUSTED':
      return t('DELIVERY_RETRY_EXHAUSTED')
    case 'DELIVERY_RETRY_SCHEDULED':
      return t('DELIVERY_RETRY_SCHEDULED')
    default:
      return errorCode
        ? t('alerts.delivery.error.unknown', { code: errorCode })
        : t('alerts.delivery.error.none')
  }
}
