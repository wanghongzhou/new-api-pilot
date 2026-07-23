import { describe, expect, test } from 'bun:test'

import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import {
  buildSettingFieldMap,
  buildSettingPatchItems,
  getMinuteRetentionDays,
  settingFieldDefinitions,
  settingsSectionKeys,
  settingsSections,
  settingsToFormValues,
} from './contract'
import {
  platformSettingKeys,
  type SettingGroup,
  type SettingsFormValues,
} from './types'

const validValues: SettingsFormValues = {
  probeIntervalMinutes: '1',
  realtimeIntervalMinutes: '1',
  resourceIntervalMinutes: '1',
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
  fastTaskHistoryRetentionHours: '24',
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
  maxFileMegabytes: '2048',
  minFreeDiskMegabytes: '5120',
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

  test('keeps the settings navigation aligned with the six API groups', () => {
    expect(settingsSections.map((section) => section.key)).toEqual([
      ...settingsSectionKeys,
    ])
  })

  test('gives every editable setting one unique form field', () => {
    const editable = settingFieldDefinitions.filter(
      (definition) => definition.kind !== 'readonly'
    )
    const formNames = editable.map((definition) => definition.formName)
    expect(formNames.every(Boolean)).toBeTrue()
    expect(new Set(formNames).size).toBe(editable.length)
  })

  test('builds an atomic changed-items patch with documented representations', () => {
    const changed: SettingsFormValues = {
      ...validValues,
      dingTalkEnabled: true,
      dingTalkSecret: 'replacement-secret',
      dingTalkSecretAction: 'replace',
      fallbackUsdExchangeRate: '7.3000',
      fastTaskHistoryRetentionHours: '48',
      maxFileMegabytes: '8796093022207',
      probeIntervalMinutes: '2',
      usageDelayMinutes: '4',
    }
    const items = buildSettingPatchItems(changed, validValues)
    expect(items).toEqual([
      { key: 'collector.probe_interval_seconds', value: 120 },
      { key: 'collector.usage_delay_minutes', value: 4 },
      { key: 'fast_task.history_retention_seconds', value: 172800 },
      { key: 'export.max_file_bytes', value: '9223372036853727232' },
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

  test('converts backend seconds and bytes into operator-facing units', () => {
    const item = (
      key: 'export.max_file_bytes' | 'export.min_free_disk_bytes',
      value: string
    ) => ({
      configured: true,
      constraints: {},
      decrypt_error: false,
      key,
      masked_value: '',
      read_only: false,
      secret: false,
      updated_at: 1,
      value,
      value_type: 'int' as const,
    })
    const values = settingsToFormValues([
      {
        items: [
          {
            configured: true,
            constraints: { maximum: 3600, minimum: 60 },
            decrypt_error: false,
            key: 'collector.probe_interval_seconds',
            masked_value: '',
            read_only: false,
            secret: false,
            updated_at: 1,
            value: 120,
            value_type: 'int',
          },
          {
            configured: true,
            constraints: {},
            decrypt_error: false,
            key: 'fast_task.history_retention_seconds',
            masked_value: '',
            read_only: false,
            secret: false,
            updated_at: 1,
            value: 86400,
            value_type: 'int',
          },
        ],
        key: 'collector',
        label_key: 'settings.groups.collector',
      },
      {
        items: [
          item('export.max_file_bytes', '2147483648'),
          item('export.min_free_disk_bytes', '5368709120'),
        ],
        key: 'export',
        label_key: 'settings.groups.export',
      },
    ])
    expect(values.probeIntervalMinutes).toBe('2')
    expect(values.fastTaskHistoryRetentionHours).toBe('24')
    expect(values.maxFileMegabytes).toBe('2048')
    expect(values.minFreeDiskMegabytes).toBe('5120')
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
