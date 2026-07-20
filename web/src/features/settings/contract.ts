import type { FieldPath } from 'react-hook-form'

import type { AnyMessageRef } from '@/lib/message-ref'

import type {
  PlatformSettingKey,
  SettingGroup,
  SettingItem,
  SettingPatchItem,
  SettingsFormValues,
  SettingsSecretState,
} from './types'

export type SettingsSectionKey =
  | 'collection'
  | 'concurrency'
  | 'export'
  | 'notification'
  | 'rate'

export type SettingControlKind =
  | 'bigint'
  | 'boolean'
  | 'decimal'
  | 'integer'
  | 'readonly'
  | 'secret'

export interface SettingFieldDefinition {
  key: PlatformSettingKey | 'system.public_origin'
  section: SettingsSectionKey
  labelKey: string
  descriptionKey: string
  kind: SettingControlKind
  formName?: FieldPath<SettingsFormValues>
  step?: string
}

export const settingFieldDefinitions: readonly SettingFieldDefinition[] = [
  {
    key: 'collector.probe_interval_seconds',
    section: 'collection',
    labelKey: 'settings.field.probeInterval',
    descriptionKey: 'settings.field.probeIntervalDescription',
    kind: 'readonly',
  },
  {
    key: 'collector.realtime_interval_seconds',
    section: 'collection',
    labelKey: 'settings.field.realtimeInterval',
    descriptionKey: 'settings.field.realtimeIntervalDescription',
    kind: 'readonly',
  },
  {
    key: 'collector.resource_interval_seconds',
    section: 'collection',
    labelKey: 'settings.field.resourceInterval',
    descriptionKey: 'settings.field.resourceIntervalDescription',
    kind: 'readonly',
  },
  {
    key: 'collector.usage_delay_minutes',
    section: 'collection',
    labelKey: 'settings.field.usageDelay',
    descriptionKey: 'settings.field.usageDelayDescription',
    kind: 'integer',
    formName: 'usageDelayMinutes',
    step: '1',
  },
  {
    key: 'collector.minute_retention_days',
    section: 'collection',
    labelKey: 'settings.field.minuteRetention',
    descriptionKey: 'settings.field.minuteRetentionDescription',
    kind: 'integer',
    formName: 'minuteRetentionDays',
    step: '1',
  },
  {
    key: 'collector.manual_backfill_max_days',
    section: 'collection',
    labelKey: 'settings.field.manualBackfillDays',
    descriptionKey: 'settings.field.manualBackfillDaysDescription',
    kind: 'integer',
    formName: 'manualBackfillMaxDays',
    step: '1',
  },
  {
    key: 'collector.probe_concurrency',
    section: 'concurrency',
    labelKey: 'settings.field.probeConcurrency',
    descriptionKey: 'settings.field.probeConcurrencyDescription',
    kind: 'integer',
    formName: 'probeConcurrency',
    step: '1',
  },
  {
    key: 'collector.realtime_concurrency',
    section: 'concurrency',
    labelKey: 'settings.field.realtimeConcurrency',
    descriptionKey: 'settings.field.realtimeConcurrencyDescription',
    kind: 'integer',
    formName: 'realtimeConcurrency',
    step: '1',
  },
  {
    key: 'collector.resource_concurrency',
    section: 'concurrency',
    labelKey: 'settings.field.resourceConcurrency',
    descriptionKey: 'settings.field.resourceConcurrencyDescription',
    kind: 'integer',
    formName: 'resourceConcurrency',
    step: '1',
  },
  {
    key: 'collector.metadata_concurrency',
    section: 'concurrency',
    labelKey: 'settings.field.metadataConcurrency',
    descriptionKey: 'settings.field.metadataConcurrencyDescription',
    kind: 'integer',
    formName: 'metadataConcurrency',
    step: '1',
  },
  {
    key: 'collector.usage_concurrency',
    section: 'concurrency',
    labelKey: 'settings.field.usageConcurrency',
    descriptionKey: 'settings.field.usageConcurrencyDescription',
    kind: 'integer',
    formName: 'usageConcurrency',
    step: '1',
  },
  {
    key: 'collector.backfill_concurrency',
    section: 'concurrency',
    labelKey: 'settings.field.backfillConcurrency',
    descriptionKey: 'settings.field.backfillConcurrencyDescription',
    kind: 'integer',
    formName: 'backfillConcurrency',
    step: '1',
  },
  {
    key: 'export.file_ttl_hours',
    section: 'export',
    labelKey: 'settings.field.fileTtl',
    descriptionKey: 'settings.field.fileTtlDescription',
    kind: 'integer',
    formName: 'fileTtlHours',
    step: '1',
  },
  {
    key: 'export.max_active_per_user',
    section: 'export',
    labelKey: 'settings.field.maxActivePerUser',
    descriptionKey: 'settings.field.maxActivePerUserDescription',
    kind: 'integer',
    formName: 'maxActivePerUser',
    step: '1',
  },
  {
    key: 'export.max_active_global',
    section: 'export',
    labelKey: 'settings.field.maxActiveGlobal',
    descriptionKey: 'settings.field.maxActiveGlobalDescription',
    kind: 'integer',
    formName: 'maxActiveGlobal',
    step: '1',
  },
  {
    key: 'export.max_file_bytes',
    section: 'export',
    labelKey: 'settings.field.maxFileBytes',
    descriptionKey: 'settings.field.maxFileBytesDescription',
    kind: 'bigint',
    formName: 'maxFileBytes',
    step: '1',
  },
  {
    key: 'export.min_free_disk_bytes',
    section: 'export',
    labelKey: 'settings.field.minFreeDiskBytes',
    descriptionKey: 'settings.field.minFreeDiskBytesDescription',
    kind: 'bigint',
    formName: 'minFreeDiskBytes',
    step: '1',
  },
  {
    key: 'rate.fallback_quota_per_unit',
    section: 'rate',
    labelKey: 'settings.field.fallbackQuotaPerUnit',
    descriptionKey: 'settings.field.fallbackQuotaPerUnitDescription',
    kind: 'decimal',
    formName: 'fallbackQuotaPerUnit',
    step: '0.0000000001',
  },
  {
    key: 'rate.fallback_usd_exchange_rate',
    section: 'rate',
    labelKey: 'settings.field.fallbackUsdExchangeRate',
    descriptionKey: 'settings.field.fallbackUsdExchangeRateDescription',
    kind: 'decimal',
    formName: 'fallbackUsdExchangeRate',
    step: '0.0000000001',
  },
  {
    key: 'system.public_origin',
    section: 'notification',
    labelKey: 'settings.field.publicOrigin',
    descriptionKey: 'settings.field.publicOriginDescription',
    kind: 'readonly',
  },
  {
    key: 'notification.dingtalk.enabled',
    section: 'notification',
    labelKey: 'settings.field.dingTalkEnabled',
    descriptionKey: 'settings.field.dingTalkEnabledDescription',
    kind: 'boolean',
    formName: 'dingTalkEnabled',
  },
  {
    key: 'notification.dingtalk.webhook',
    section: 'notification',
    labelKey: 'settings.field.dingTalkWebhook',
    descriptionKey: 'settings.field.dingTalkWebhookDescription',
    kind: 'secret',
    formName: 'dingTalkWebhook',
  },
  {
    key: 'notification.dingtalk.secret',
    section: 'notification',
    labelKey: 'settings.field.dingTalkSecret',
    descriptionKey: 'settings.field.dingTalkSecretDescription',
    kind: 'secret',
    formName: 'dingTalkSecret',
  },
]

export const settingsSections: ReadonlyArray<{
  key: SettingsSectionKey
  titleKey: string
  descriptionKey: string
}> = [
  {
    key: 'collection',
    titleKey: 'settings.section.collection',
    descriptionKey: 'settings.section.collectionDescription',
  },
  {
    key: 'concurrency',
    titleKey: 'settings.section.concurrency',
    descriptionKey: 'settings.section.concurrencyDescription',
  },
  {
    key: 'export',
    titleKey: 'settings.section.export',
    descriptionKey: 'settings.section.exportDescription',
  },
  {
    key: 'rate',
    titleKey: 'settings.section.rate',
    descriptionKey: 'settings.section.rateDescription',
  },
  {
    key: 'notification',
    titleKey: 'settings.section.notification',
    descriptionKey: 'settings.section.notificationDescription',
  },
]

const editableDefinitions = settingFieldDefinitions.filter(
  (
    definition
  ): definition is SettingFieldDefinition & {
    key: PlatformSettingKey
    formName: FieldPath<SettingsFormValues>
  } => definition.formName != null && definition.kind !== 'readonly'
)

export const emptySettingsFormValues: SettingsFormValues = {
  usageDelayMinutes: '',
  minuteRetentionDays: '',
  probeConcurrency: '',
  realtimeConcurrency: '',
  resourceConcurrency: '',
  metadataConcurrency: '',
  usageConcurrency: '',
  backfillConcurrency: '',
  manualBackfillMaxDays: '',
  fileTtlHours: '',
  maxActivePerUser: '',
  maxActiveGlobal: '',
  maxFileBytes: '',
  minFreeDiskBytes: '',
  fallbackQuotaPerUnit: '',
  fallbackUsdExchangeRate: '',
  dingTalkEnabled: false,
  dingTalkWebhook: '',
  dingTalkWebhookAction: 'keep',
  dingTalkSecret: '',
  dingTalkSecretAction: 'keep',
}

export function settingItemsByKey(
  groups: readonly SettingGroup[] | undefined
): Map<string, SettingItem> {
  const result = new Map<string, SettingItem>()
  for (const group of groups ?? []) {
    for (const item of group.items) result.set(item.key, item)
  }
  return result
}

function editableValue(item: SettingItem | undefined): string | boolean {
  if (item?.value_type === 'bool') return item.value === true
  if (typeof item?.value === 'number') return String(item.value)
  if (typeof item?.value === 'string') return item.value
  return ''
}

export function settingsToFormValues(
  groups: readonly SettingGroup[]
): SettingsFormValues {
  const items = settingItemsByKey(groups)
  const values = { ...emptySettingsFormValues }
  for (const definition of editableDefinitions) {
    if (definition.kind === 'secret') continue
    const value = editableValue(items.get(definition.key))
    ;(values as Record<string, string | boolean>)[definition.formName] = value
  }
  return values
}

export function settingsSecretState(
  groups: readonly SettingGroup[] | undefined
): SettingsSecretState {
  const items = settingItemsByKey(groups)
  const webhook = items.get('notification.dingtalk.webhook')
  const secret = items.get('notification.dingtalk.secret')
  return {
    webhook: {
      configured: webhook?.configured === true,
      decryptError: webhook?.decrypt_error === true,
    },
    secret: {
      configured: secret?.configured === true,
      decryptError: secret?.decrypt_error === true,
    },
  }
}

const settingKeyToFormName = Object.fromEntries(
  editableDefinitions.map((definition) => [definition.key, definition.formName])
) as Partial<Record<PlatformSettingKey, FieldPath<SettingsFormValues>>>

function secretPatch(
  key: 'notification.dingtalk.secret' | 'notification.dingtalk.webhook',
  action: SettingsFormValues['dingTalkSecretAction' | 'dingTalkWebhookAction'],
  value: string
): SettingPatchItem | null {
  if (action === 'clear') return { clear: true, key }
  if (action === 'replace') return { key, value }
  return null
}

export function buildSettingPatchItems(
  values: SettingsFormValues,
  initial: SettingsFormValues
): SettingPatchItem[] {
  const result: SettingPatchItem[] = []
  for (const definition of editableDefinitions) {
    if (definition.kind === 'secret') continue
    const current = values[definition.formName as keyof SettingsFormValues]
    const previous = initial[definition.formName as keyof SettingsFormValues]
    if (current === previous) continue
    let value: boolean | number | string = String(current)
    if (definition.kind === 'boolean') value = Boolean(current)
    else if (definition.kind === 'integer') value = Number(current)
    result.push({
      key: definition.key,
      value,
    })
  }
  const webhookPatch = secretPatch(
    'notification.dingtalk.webhook',
    values.dingTalkWebhookAction,
    values.dingTalkWebhook
  )
  const secretValuePatch = secretPatch(
    'notification.dingtalk.secret',
    values.dingTalkSecretAction,
    values.dingTalkSecret
  )
  if (webhookPatch) result.push(webhookPatch)
  if (secretValuePatch) result.push(secretValuePatch)
  return result
}

export function buildSettingFieldMap(
  items: readonly SettingPatchItem[]
): Readonly<Record<string, FieldPath<SettingsFormValues>>> {
  const result: Record<string, FieldPath<SettingsFormValues>> = {}
  for (const [key, field] of Object.entries(settingKeyToFormName)) {
    if (field) result[key] = field
  }
  items.forEach((item, index) => {
    const field = settingKeyToFormName[item.key]
    if (!field) return
    result[`items[${index}].key`] = field
    result[`items[${index}].value`] = field
    result[`items[${index}].clear`] = field
  })
  return result
}

export function getMinuteRetentionDays(
  groups: readonly SettingGroup[] | undefined
): number | null {
  const value = settingItemsByKey(groups).get(
    'collector.minute_retention_days'
  )?.value
  return typeof value === 'number' && Number.isSafeInteger(value) && value > 0
    ? value
    : null
}

export function buildSettingSLOMessageRefs(
  groups: readonly SettingGroup[] | undefined,
  values: SettingsFormValues
): AnyMessageRef[] {
  const codes = groups?.[0]?.h15_slo_reason_codes ?? []
  const unique = new Set(codes)
  const result: AnyMessageRef[] = []
  for (const code of unique) {
    if (code === 'SLO_USAGE_DELAY_TOO_HIGH') {
      result.push({
        code,
        params: { threshold: 5, value: Number(values.usageDelayMinutes) },
        technical_detail: '',
      })
    } else if (code === 'SLO_USAGE_CONCURRENCY_TOO_LOW') {
      result.push({
        code,
        params: { threshold: 5, value: Number(values.usageConcurrency) },
        technical_detail: '',
      })
    }
  }
  return result
}
