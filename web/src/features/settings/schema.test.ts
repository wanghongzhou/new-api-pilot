import { describe, expect, test } from 'bun:test'

import { createSettingsFormSchema } from './schema'
import type { SettingsFormValues, SettingsSecretState } from './types'

const availableSecrets: SettingsSecretState = {
  secret: { configured: true, decryptError: false },
  webhook: { configured: true, decryptError: false },
}

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

describe('settings form schema', () => {
  test('accepts the canonical scalar contract', () => {
    expect(
      createSettingsFormSchema(availableSecrets).safeParse(validValues).success
    ).toBeTrue()
  })

  test.each([
    ['ordinary exponent', { usageDelayMinutes: '5e0' }],
    ['ordinary leading zero', { usageDelayMinutes: '05' }],
    ['bigint exponent', { maxFileBytes: '2e10' }],
    ['bigint overflow', { maxFileBytes: '9223372036854775808' }],
    ['decimal exponent', { fallbackUsdExchangeRate: '7.3e0' }],
    ['decimal sign', { fallbackUsdExchangeRate: '+7.3' }],
    ['decimal zero', { fallbackUsdExchangeRate: '0.000' }],
    ['decimal scale', { fallbackUsdExchangeRate: '1.12345678901' }],
    ['per-user exceeds global', { maxActivePerUser: '11' }],
  ])('rejects %s', (_name, changed) => {
    expect(
      createSettingsFormSchema(availableSecrets).safeParse({
        ...validValues,
        ...changed,
      }).success
    ).toBeFalse()
  })

  test('requires both usable secrets when enabling DingTalk', () => {
    const unavailable: SettingsSecretState = {
      secret: { configured: false, decryptError: false },
      webhook: { configured: false, decryptError: false },
    }
    expect(
      createSettingsFormSchema(unavailable).safeParse({
        ...validValues,
        dingTalkEnabled: true,
      }).success
    ).toBeFalse()
    expect(
      createSettingsFormSchema(unavailable).safeParse({
        ...validValues,
        dingTalkEnabled: true,
        dingTalkSecret: 'new-secret',
        dingTalkSecretAction: 'replace',
        dingTalkWebhook: 'https://oapi.dingtalk.com/robot/send?token=test',
        dingTalkWebhookAction: 'replace',
      }).success
    ).toBeTrue()
  })

  test('forces decrypt errors to replace or disabled-and-clear', () => {
    const broken: SettingsSecretState = {
      secret: { configured: true, decryptError: true },
      webhook: { configured: true, decryptError: false },
    }
    expect(
      createSettingsFormSchema(broken).safeParse(validValues).success
    ).toBeFalse()
    expect(
      createSettingsFormSchema(broken).safeParse({
        ...validValues,
        dingTalkEnabled: false,
        dingTalkSecretAction: 'clear',
      }).success
    ).toBeTrue()
  })
})
