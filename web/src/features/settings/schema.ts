import { z } from 'zod'

import type { SettingsFormValues, SettingsSecretState } from './types'

const positiveInteger = (minimum: number, maximum: number) =>
  z
    .string()
    .regex(/^[1-9]\d*$/, { message: 'settings.validation.integer' })
    .refine((value) => {
      const parsed = Number(value)
      return (
        Number.isSafeInteger(parsed) && parsed >= minimum && parsed <= maximum
      )
    }, 'settings.validation.range')

const positiveInt64String = z
  .string()
  .regex(/^[1-9]\d*$/, { message: 'settings.validation.bigint' })
  .refine(
    (value) =>
      /^[1-9]\d*$/.test(value) && BigInt(value) <= 9_223_372_036_854_775_807n,
    { message: 'settings.validation.bigint' }
  )

export function isOptionalPositiveFixedDecimal(value: string): boolean {
  if (value === '') return true
  if (!/^\d+(?:\.\d+)?$/.test(value)) return false
  const [rawInteger, fraction = ''] = value.split('.')
  if (fraction.length > 10) return false
  const integer = rawInteger.replace(/^0+/, '')
  const integerDigits = integer.length
  if (integerDigits > 20 || integerDigits + fraction.length > 30) return false
  return /[1-9]/.test(`${integer}${fraction}`)
}

const optionalPositiveDecimal = z
  .string()
  .refine(isOptionalPositiveFixedDecimal, 'settings.validation.decimal')

const secretAction = z.enum(['clear', 'keep', 'replace'])

const runtimeList = z.string().max(8192)

function validHttpsWebhook(value: string): boolean {
  if (value.length < 1 || value.length > 4096) return false
  try {
    return new URL(value).protocol === 'https:'
  } catch {
    return false
  }
}

function containsUnsafeSecretCharacter(value: string): boolean {
  for (const character of value) {
    if (character === '\0' || character === '\r' || character === '\n') {
      return true
    }
  }
  return false
}

function addSecretAvailabilityIssue(
  context: z.RefinementCtx,
  path: keyof SettingsFormValues
) {
  context.addIssue({
    code: 'custom',
    message: 'settings.validation.notificationRequired',
    path: [path],
  })
}

export function createSettingsFormSchema(secretState: SettingsSecretState) {
  return z
    .object({
      usageDelayMinutes: positiveInteger(1, 59),
      minuteRetentionDays: positiveInteger(1, 3650),
      logRetentionDays: positiveInteger(1, 3650),
      performanceRetentionDays: positiveInteger(1, 3650),
      taskRetentionDays: positiveInteger(1, 3650),
      systemTaskTerminalRetentionDays: positiveInteger(1, 3650),
      probeConcurrency: positiveInteger(1, 100),
      realtimeConcurrency: positiveInteger(1, 100),
      resourceConcurrency: positiveInteger(1, 100),
      metadataConcurrency: positiveInteger(1, 100),
      usageConcurrency: positiveInteger(1, 100),
      backfillConcurrency: positiveInteger(1, 100),
      manualBackfillMaxDays: positiveInteger(1, 3660),
      fastTaskHistoryRetentionSeconds: positiveInteger(60, 31_536_000),
      fastTaskHistoryCount: positiveInteger(1, 1000),
      upstreamAllowedHostSuffixes: runtimeList,
      upstreamAllowedCidrs: runtimeList,
      upstreamConnectTimeoutSeconds: positiveInteger(1, 60),
      upstreamResponseHeaderTimeoutSeconds: positiveInteger(1, 300),
      upstreamRequestTimeoutSeconds: positiveInteger(1, 600),
      upstreamExportTimeoutSeconds: positiveInteger(1, 3600),
      upstreamRateLimitRequests: positiveInteger(1, 10_000),
      upstreamRateLimitWindowSeconds: positiveInteger(1, 3600),
      upstreamMaxInflightPerOrigin: positiveInteger(1, 100),
      fileTtlHours: positiveInteger(1, 168),
      maxActivePerUser: positiveInteger(1, 100),
      maxActiveGlobal: positiveInteger(1, 100),
      maxFileBytes: positiveInt64String,
      minFreeDiskBytes: positiveInt64String,
      fallbackQuotaPerUnit: optionalPositiveDecimal,
      fallbackUsdExchangeRate: optionalPositiveDecimal,
      dingTalkEnabled: z.boolean(),
      dingTalkWebhook: z.string().max(4096),
      dingTalkWebhookAction: secretAction,
      dingTalkSecret: z.string().max(1024),
      dingTalkSecretAction: secretAction,
    })
    .superRefine((values, context) => {
      if (Number(values.maxActivePerUser) > Number(values.maxActiveGlobal)) {
        context.addIssue({
          code: 'custom',
          message: 'settings.validation.perUserLimit',
          path: ['maxActivePerUser'],
        })
      }

      const requestTimeout = Number(values.upstreamRequestTimeoutSeconds)
      if (Number(values.upstreamConnectTimeoutSeconds) > requestTimeout) {
        context.addIssue({
          code: 'custom',
          message: 'settings.validation.timeoutRelationship',
          path: ['upstreamConnectTimeoutSeconds'],
        })
      }
      if (
        Number(values.upstreamResponseHeaderTimeoutSeconds) > requestTimeout
      ) {
        context.addIssue({
          code: 'custom',
          message: 'settings.validation.timeoutRelationship',
          path: ['upstreamResponseHeaderTimeoutSeconds'],
        })
      }
      if (requestTimeout > Number(values.upstreamExportTimeoutSeconds)) {
        context.addIssue({
          code: 'custom',
          message: 'settings.validation.timeoutRelationship',
          path: ['upstreamRequestTimeoutSeconds'],
        })
      }
      if (
        (Number(values.upstreamRateLimitWindowSeconds) * 1000) /
          Number(values.upstreamRateLimitRequests) <
        10
      ) {
        context.addIssue({
          code: 'custom',
          message: 'settings.validation.rateInterval',
          path: ['upstreamRateLimitRequests'],
        })
      }

      if (
        values.dingTalkWebhookAction === 'replace' &&
        !validHttpsWebhook(values.dingTalkWebhook)
      ) {
        context.addIssue({
          code: 'custom',
          message: 'settings.validation.webhook',
          path: ['dingTalkWebhook'],
        })
      }
      if (
        values.dingTalkSecretAction === 'replace' &&
        (values.dingTalkSecret.length < 1 ||
          containsUnsafeSecretCharacter(values.dingTalkSecret))
      ) {
        context.addIssue({
          code: 'custom',
          message: 'settings.validation.secret',
          path: ['dingTalkSecret'],
        })
      }
      if (
        secretState.webhook.decryptError &&
        values.dingTalkWebhookAction === 'keep'
      ) {
        context.addIssue({
          code: 'custom',
          message: 'settings.validation.decryptResolution',
          path: ['dingTalkWebhook'],
        })
      }
      if (
        secretState.secret.decryptError &&
        values.dingTalkSecretAction === 'keep'
      ) {
        context.addIssue({
          code: 'custom',
          message: 'settings.validation.decryptResolution',
          path: ['dingTalkSecret'],
        })
      }

      if (!values.dingTalkEnabled) return
      const webhookAvailable =
        values.dingTalkWebhookAction === 'replace'
          ? validHttpsWebhook(values.dingTalkWebhook)
          : values.dingTalkWebhookAction === 'keep' &&
            secretState.webhook.configured &&
            !secretState.webhook.decryptError
      const secretAvailable =
        values.dingTalkSecretAction === 'replace'
          ? values.dingTalkSecret.length > 0 &&
            !containsUnsafeSecretCharacter(values.dingTalkSecret)
          : values.dingTalkSecretAction === 'keep' &&
            secretState.secret.configured &&
            !secretState.secret.decryptError
      if (!webhookAvailable) {
        addSecretAvailabilityIssue(context, 'dingTalkWebhook')
      }
      if (!secretAvailable) {
        addSecretAvailabilityIssue(context, 'dingTalkSecret')
      }
    }) satisfies z.ZodType<SettingsFormValues>
}
