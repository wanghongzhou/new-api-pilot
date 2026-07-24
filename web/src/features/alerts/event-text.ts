import { fromUnixSeconds } from '@/lib/dayjs'

import { alertTargetTypeText } from './components/alert-ui'
import type { AlertEventItem } from './types'

export function alertEventTargetLabel(
  t: (key: string) => string,
  event: Pick<AlertEventItem, 'rule_key' | 'target_type'>
): string {
  if (event.target_type !== 'collection') {
    return alertTargetTypeText(t, event.target_type)
  }
  return event.rule_key === 'backfill_failed'
    ? t('alerts.target.backfillRun')
    : t('alerts.target.collectionWindow')
}

export function alertEventTargetName(
  t: (key: string, params?: Record<string, unknown>) => string,
  event: Pick<
    AlertEventItem,
    'rule_key' | 'target_key' | 'target_name' | 'target_type'
  >
): string {
  if (event.target_type !== 'collection') return event.target_name
  const identifier = event.target_key.split('/').at(-1)
  if (!identifier) return event.target_name
  if (event.rule_key === 'backfill_failed') {
    return t('alerts.target.backfillRunValue', { id: identifier })
  }
  const hourTimestamp = Number(identifier)
  if (!Number.isSafeInteger(hourTimestamp) || hourTimestamp <= 0) {
    return event.target_name
  }
  return t('alerts.target.collectionWindowValue', {
    time: fromUnixSeconds(hourTimestamp).format('YYYY-MM-DD HH:00'),
  })
}
