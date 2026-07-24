import Decimal from 'decimal.js'

import type { AnyMessageRef } from '@/lib/message-ref'

import type { AlertRuleCategory } from './types'

export function formatAlertThreshold(value: string): string {
  try {
    return new Decimal(value).toDecimalPlaces(2).toString()
  } catch {
    return value
  }
}

export function formatAlertCurrentValue(value: string): string {
  try {
    return new Decimal(value).toDecimalPlaces(2).toString()
  } catch {
    return value
  }
}

export function alertMessageForDisplay(message: AnyMessageRef): AnyMessageRef {
  const params = { ...message.params } as Record<string, unknown>
  if (typeof params.value === 'string') {
    params.value = formatAlertCurrentValue(params.value)
  }
  if (typeof params.threshold === 'string') {
    params.threshold = formatAlertThreshold(params.threshold)
  }
  return { ...message, params } as AnyMessageRef
}

export function alertRuleCategoryText(
  t: (key: string) => string,
  category: AlertRuleCategory
): string {
  switch (category) {
    case 'site':
      return t('alerts.rules.category.site')
    case 'collection':
      return t('alerts.rules.category.collection')
    case 'instance':
      return t('alerts.rules.category.instance')
    case 'account':
      return t('alerts.rules.category.account')
    case 'channel':
      return t('alerts.rules.category.channel')
  }
}
