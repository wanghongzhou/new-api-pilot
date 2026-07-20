import { describe, expect, test } from 'bun:test'

import {
  buildSettingFieldMap,
  buildSettingPatchItems,
  buildSettingSLOMessageRefs,
  getMinuteRetentionDays,
  settingFieldDefinitions,
} from './contract'
import type { SettingGroup, SettingsFormValues } from './types'
import { platformSettingKeys } from './types'

const validValues: SettingsFormValues = {
  usageDelayMinutes: '5',
  minuteRetentionDays: '90',
  probeConcurrency: '20',
  realtimeConcurrency: '10',
  resourceConcurrency: '10',
  metadataConcurrency: '5',
  usageConcurrency: '5',
  backfillConcurrency: '2',
  manualBackfillMaxDays: '366',
  fileTtlHours: '24',
  maxActivePerUser: '3',
  maxActiveGlobal: '10',
  maxFileBytes: '2147483648',
  minFreeDiskBytes: '5368709120',
  fallbackQuotaPerUnit: '',
  fallbackUsdExchangeRate: '7.3',
  dingTalkEnabled: false,
  dingTalkWebhook: '',
  dingTalkWebhookAction: 'keep',
  dingTalkSecret: '',
  dingTalkSecretAction: 'keep',
}

function groupsFixture(retentionValue: unknown = 90): SettingGroup[] {
  return [
    {
      h15_slo_eligible: false,
      h15_slo_reason_codes: [
        'SLO_USAGE_DELAY_TOO_HIGH',
        'SLO_USAGE_CONCURRENCY_TOO_LOW',
      ],
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
  test('covers each of the 22 persisted settings exactly once', () => {
    expect(platformSettingKeys).toHaveLength(22)
    expect(new Set(platformSettingKeys).size).toBe(22)
    const renderedPersistedKeys = settingFieldDefinitions
      .map((definition) => definition.key)
      .filter((key) => key !== 'system.public_origin')
    expect(new Set(renderedPersistedKeys).size).toBe(22)
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

  test('constructs complete H+15 MessageRefs from machine flags', () => {
    const refs = buildSettingSLOMessageRefs(groupsFixture(), {
      ...validValues,
      usageConcurrency: '3',
      usageDelayMinutes: '12',
    })
    expect(refs).toEqual([
      {
        code: 'SLO_USAGE_DELAY_TOO_HIGH',
        params: { threshold: 5, value: 12 },
        technical_detail: '',
      },
      {
        code: 'SLO_USAGE_CONCURRENCY_TOO_LOW',
        params: { threshold: 5, value: 3 },
        technical_detail: '',
      },
    ])
  })

  test('returns dynamic minute retention only for a valid Viewer setting', () => {
    expect(getMinuteRetentionDays(groupsFixture(37))).toBe(37)
    expect(getMinuteRetentionDays(groupsFixture('37'))).toBeNull()
    expect(getMinuteRetentionDays(undefined)).toBeNull()
  })
})
