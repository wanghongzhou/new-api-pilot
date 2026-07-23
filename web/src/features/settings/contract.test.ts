import { describe, expect, test } from 'bun:test'

import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import {
  buildSettingFieldMap,
  buildSettingPatchItems,
  getMinuteRetentionDays,
  settingFieldDefinitions,
  settingsSections,
} from './contract'
import {
  platformSettingKeys,
  type SettingGroup,
  type SettingsFormValues,
} from './types'

const validValues: SettingsFormValues = {
  usageDelayMinutes: '5',
  minuteRetentionDays: '90',
  logRetentionDays: '90',
  performanceRetentionDays: '90',
  taskRetentionDays: '90',
  systemTaskTerminalRetentionDays: '90',
  probeConcurrency: '20',
  realtimeConcurrency: '10',
  resourceConcurrency: '10',
  metadataConcurrency: '5',
  usageConcurrency: '5',
  backfillConcurrency: '2',
  manualBackfillMaxDays: '366',
  fastTaskHistoryRetentionSeconds: '86400',
  fastTaskHistoryCount: '100',
  upstreamAllowedHostSuffixes: '',
  upstreamAllowedCidrs: '',
  upstreamConnectTimeoutSeconds: '5',
  upstreamResponseHeaderTimeoutSeconds: '15',
  upstreamRequestTimeoutSeconds: '30',
  upstreamExportTimeoutSeconds: '120',
  upstreamRateLimitRequests: '300',
  upstreamRateLimitWindowSeconds: '180',
  upstreamMaxInflightPerOrigin: '4',
  fileTtlHours: '24',
  maxActivePerUser: '3',
  maxActiveGlobal: '10',
  maxFileBytes: '2147483648',
  minFreeDiskBytes: '5368709120',
  fallbackQuotaPerUnit: '500000',
  fallbackUsdExchangeRate: '6.8',
  dingTalkEnabled: false,
  dingTalkWebhook: '',
  dingTalkWebhookAction: 'keep',
  dingTalkSecret: '',
  dingTalkSecretAction: 'keep',
}

function groupsFixture(retentionValue: unknown = 90): SettingGroup[] {
  return [
    {
      items: [
        {
          configured: true,
          constraints: { maximum: 3650, minimum: 1 },
          decrypt_error: false,
          key: 'collector.minute_retention_days',
          masked_value: '',
          read_only: false,
          secret: false,
          updated_at: 1,
          value: retentionValue as number,
          value_type: 'int',
        },
      ],
      key: 'collector',
      label_key: 'settings.groups.collector',
    },
  ]
}

describe('settings frontend contract', () => {
  test('covers each of the 37 persisted settings exactly once', () => {
    expect(platformSettingKeys).toHaveLength(37)
    expect(new Set(platformSettingKeys).size).toBe(37)
    const renderedPersistedKeys = settingFieldDefinitions
      .map((definition) => definition.key)
      .filter((key) => key !== 'system.public_origin')
    expect(new Set(renderedPersistedKeys).size).toBe(37)
    expect([...renderedPersistedKeys].sort()).toEqual(
      [...platformSettingKeys].sort()
    )
  })

  test('gives every rendered setting a dedicated explanatory translation key', () => {
    const descriptions = settingFieldDefinitions.map(
      (definition) => definition.descriptionKey
    )
    expect(new Set(descriptions).size).toBe(settingFieldDefinitions.length)
  })

  test('registers every dynamically rendered setting translation key', () => {
    for (const definition of settingFieldDefinitions) {
      expect(dynamicI18nKey('settings', definition.labelKey)).toBe(
        definition.labelKey
      )
      expect(dynamicI18nKey('settings', definition.descriptionKey)).toBe(
        definition.descriptionKey
      )
    }
    for (const section of settingsSections) {
      expect(dynamicI18nKey('settings', section.titleKey)).toBe(
        section.titleKey
      )
      expect(dynamicI18nKey('settings', section.descriptionKey)).toBe(
        section.descriptionKey
      )
    }
  })

  test('builds an atomic changed-items patch with documented representations', () => {
    const changed: SettingsFormValues = {
      ...validValues,
      dingTalkEnabled: true,
      dingTalkSecret: 'replacement-secret',
      dingTalkSecretAction: 'replace',
      fallbackUsdExchangeRate: '7.3000',
      maxFileBytes: '9007199254740993',
      usageDelayMinutes: '4',
    }
    const items = buildSettingPatchItems(changed, validValues)
    expect(items).toEqual([
      { key: 'collector.usage_delay_minutes', value: 4 },
      { key: 'export.max_file_bytes', value: '9007199254740993' },
      { key: 'rate.fallback_usd_exchange_rate', value: '7.3000' },
      { key: 'notification.dingtalk.enabled', value: true },
      {
        key: 'notification.dingtalk.secret',
        value: 'replacement-secret',
      },
    ])
    expect(JSON.stringify(items)).not.toContain('updated_at')
    expect(JSON.stringify(items)).not.toContain('version')
  })

  test('maps indexed and final-state server errors to the edited controls', () => {
    const items = buildSettingPatchItems(
      { ...validValues, maxActivePerUser: '8' },
      validValues
    )
    expect(buildSettingFieldMap(items)).toMatchObject({
      'export.max_active_per_user': 'maxActivePerUser',
      'items[0].key': 'maxActivePerUser',
      'items[0].value': 'maxActivePerUser',
    })
  })

  test('returns dynamic minute retention only for a valid Viewer setting', () => {
    expect(getMinuteRetentionDays(groupsFixture(37))).toBe(37)
    expect(getMinuteRetentionDays(groupsFixture('37'))).toBeNull()
    expect(getMinuteRetentionDays(undefined)).toBeNull()
  })
})
