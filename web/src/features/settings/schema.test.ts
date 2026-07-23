import { describe, expect, test } from 'bun:test'

import { createSettingsFormSchema } from './schema'
import type { SettingsFormValues, SettingsSecretState } from './types'

const availableSecrets: SettingsSecretState = {
  secret: { configured: true, decryptError: false },
  webhook: { configured: true, decryptError: false },
}

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

describe('settings form schema', () => {
  test('accepts the canonical scalar contract', () => {
    expect(
      createSettingsFormSchema(availableSecrets).safeParse(validValues).success
    ).toBeTrue()
  })

  test.each([
    ['collector interval zero', { probeIntervalMinutes: '0' }],
    ['collector interval above one hour', { resourceIntervalMinutes: '61' }],
    ['ordinary exponent', { usageDelayMinutes: '5e0' }],
    ['ordinary leading zero', { usageDelayMinutes: '05' }],
    ['megabyte exponent', { maxFileMegabytes: '2e10' }],
    ['megabyte overflow', { maxFileMegabytes: '8796093022208' }],
    ['retention below one minute', { fastTaskHistoryRetentionHours: '0.01' }],
    ['retention precision', { fastTaskHistoryRetentionHours: '1.12345' }],
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
