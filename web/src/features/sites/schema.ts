import { stripSearchParams } from '@tanstack/react-router'
import { z } from 'zod'

import {
  collectionRunStatuses,
  collectionTaskTypes,
  collectionRunWindowStatuses,
  siteAuthStatuses,
  siteHealthStatuses,
  siteManagementStatuses,
  siteOnlineStatuses,
  siteStatisticsStatuses,
} from './constants'

const idStringSchema = z.coerce.string().regex(/^[1-9]\d*$/, 'Invalid ID')

function searchArray<T extends readonly [string, ...string[]]>(values: T) {
  return z
    .preprocess(
      (value) => {
        if (value == null) return []
        return Array.isArray(value) ? value : [value]
      },
      z.array(z.enum(values))
    )
    .catch([])
}

export function normalizeBaseUrl(value: string): string {
  const url = new URL(value.trim())
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new TypeError('Base URL must use HTTP or HTTPS')
  }
  if (url.username || url.password || url.search || url.hash) {
    throw new TypeError('Base URL cannot include credentials, query, or hash')
  }

  url.pathname = url.pathname.replace(/\/+$/, '')
  return url.toString().replace(/\/$/, '')
}

export const baseUrlSchema = z
  .string()
  .trim()
  .min(1, 'Base URL is required')
  .max(255, 'Base URL is too long')
  .transform((value, context) => {
    try {
      return normalizeBaseUrl(value)
    } catch {
      context.addIssue({ code: 'custom', message: 'Enter a valid base URL' })
      return z.NEVER
    }
  })

export const sitesSearchSchema = z.object({
  page: z.coerce.number().int().min(1).optional().catch(undefined),
  pageSize: z.coerce.number().int().min(1).max(100).optional().catch(undefined),
  filter: z.string().max(128).optional().catch(undefined),
  management: searchArray(siteManagementStatuses),
  online: searchArray(siteOnlineStatuses),
  auth: searchArray(siteAuthStatuses),
  statistics: searchArray(siteStatisticsStatuses),
  health: searchArray(siteHealthStatuses),
  view: z.enum(['card', 'table']).optional().catch(undefined),
  sort: z
    .enum(['priority', 'name', 'today_quota', 'updated_at'])
    .optional()
    .catch(undefined),
  order: z.enum(['asc', 'desc']).optional().catch(undefined),
})

type SitesSearchParams = z.output<typeof sitesSearchSchema>

export const siteSearchMiddlewares = [
  stripSearchParams<SitesSearchParams>({
    auth: [],
    health: [],
    management: [],
    online: [],
    statistics: [],
  }),
]

export const siteDetailSearchSchema = z.object({
  runId: idStringSchema.optional().catch(undefined),
  runPage: z.coerce.number().int().min(1).optional().catch(undefined),
  runStatus: z.enum(collectionRunStatuses).optional().catch(undefined),
  runTaskType: z.enum(collectionTaskTypes).optional().catch(undefined),
  windowPage: z.coerce.number().int().min(1).optional().catch(undefined),
  windowStatus: z.enum(collectionRunWindowStatuses).optional().catch(undefined),
})

export const siteStatusSearchSchema = z.object({
  start: z.coerce.number().int().positive().optional().catch(undefined),
  end: z.coerce.number().int().positive().optional().catch(undefined),
  granularity: z.enum(['minute', 'hour', 'day']).optional().catch(undefined),
  nodeName: z.string().max(128).optional().catch(undefined),
  metric: z.enum(['cpu', 'memory', 'disk']).optional().catch(undefined),
  aggregation: z.enum(['max', 'avg', 'last']).optional().catch(undefined),
})

export const runWindowSearchSchema = z.object({
  page: z.coerce.number().int().min(1).optional().catch(undefined),
  status: z.enum(collectionRunWindowStatuses).optional().catch(undefined),
})

export const siteFormSchema = z.object({
  name: z.string().trim().min(1, 'Site name is required').max(128),
  baseUrl: baseUrlSchema,
  remark: z.string().trim().max(500).optional(),
})

export const existingTokenAuthorizationSchema = z.object({
  mode: z.literal('existing_token'),
  rootUserId: idStringSchema,
  accessToken: z.string().min(1, 'Access token is required').max(4096),
})

export const loginAuthorizationSchema = z.object({
  mode: z.literal('login_generate_token'),
  username: z.string().trim().min(1, 'Username is required').max(128),
  password: z.string().min(1, 'Password is required').max(1024),
  confirmTokenRotation: z.literal(true, {
    error: 'Confirm token rotation',
  }),
})

export const siteAuthorizationSchema = z.discriminatedUnion('mode', [
  existingTokenAuthorizationSchema,
  loginAuthorizationSchema,
])

export const siteAuthorizationFormSchema = z
  .object({
    mode: z.enum(['existing_token', 'login_generate_token']),
    rootUserId: z.string().optional(),
    accessToken: z.string().max(4096).optional(),
    username: z.string().trim().max(128).optional(),
    password: z.string().max(1024).optional(),
    confirmTokenRotation: z.boolean().optional(),
  })
  .superRefine((value, context) => {
    if (value.mode === 'existing_token') {
      if (!value.rootUserId || !/^[1-9]\d*$/.test(value.rootUserId)) {
        context.addIssue({
          code: 'custom',
          message: 'Invalid ID',
          path: ['rootUserId'],
        })
      }
      if (!value.accessToken) {
        context.addIssue({
          code: 'custom',
          message: 'Access token is required',
          path: ['accessToken'],
        })
      }
      return
    }

    if (!value.username) {
      context.addIssue({
        code: 'custom',
        message: 'Username is required',
        path: ['username'],
      })
    }
    if (!value.password) {
      context.addIssue({
        code: 'custom',
        message: 'Password is required',
        path: ['password'],
      })
    }
    if (value.confirmTokenRotation !== true) {
      context.addIssue({
        code: 'custom',
        message: 'Confirm token rotation',
        path: ['confirmTokenRotation'],
      })
    }
  })

export const siteBackfillSchema = z
  .object({
    startTimestamp: z.number().int().positive().optional(),
    endTimestamp: z.number().int().positive().optional(),
    onlyMissing: z.boolean().default(true),
  })
  .refine(
    (value) =>
      value.startTimestamp == null ||
      value.endTimestamp == null ||
      value.endTimestamp > value.startTimestamp,
    { message: 'End time must be after start time', path: ['endTimestamp'] }
  )

export const siteStatisticsEndSchema = z.object({
  statisticsEndAt: z.number().int().positive(),
})

export type SiteFormValues = z.input<typeof siteFormSchema>
export type SiteFormOutput = z.output<typeof siteFormSchema>
export type SiteAuthorizationValues = z.input<
  typeof siteAuthorizationFormSchema
>
export type SiteBackfillValues = z.input<typeof siteBackfillSchema>
export type SiteStatisticsEndValues = z.input<typeof siteStatisticsEndSchema>
