import { z } from 'zod'

import { isIdString, isNonNegativeIdString } from '@/lib/api-types'

import { buildUpstreamTaskSearch, upstreamTaskStatuses } from './search'

function arrayValue(value: unknown) {
  if (value == null) return []
  return Array.isArray(value) ? value : [value]
}

const strings = (maximum: number) =>
  z.preprocess(arrayValue, z.array(z.string()).max(maximum)).catch([])

export const upstreamTaskSearchSchema = z
  .object({
    actions: strings(100),
    end: z.coerce.number().int().positive().optional().catch(undefined),
    exportId: z.string().refine(isIdString).optional().catch(undefined),
    groups: strings(100),
    models: strings(100),
    page: z.coerce.number().int().min(1).optional().catch(undefined),
    pageSize: z.coerce
      .number()
      .int()
      .min(1)
      .max(100)
      .optional()
      .catch(undefined),
    platforms: strings(100),
    remoteChannelId: z
      .string()
      .refine(isNonNegativeIdString)
      .optional()
      .catch(undefined),
    remoteId: z.string().refine(isIdString).optional().catch(undefined),
    remoteUserId: z
      .string()
      .refine(isNonNegativeIdString)
      .optional()
      .catch(undefined),
    siteIds: strings(100),
    start: z.coerce.number().int().positive().optional().catch(undefined),
    statuses: z
      .preprocess(arrayValue, z.array(z.enum(upstreamTaskStatuses)).max(20))
      .catch([]),
    taskId: z.string().optional().catch(undefined),
  })
  .transform((search) => buildUpstreamTaskSearch(search))
