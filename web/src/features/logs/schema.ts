import { z } from 'zod'

import { isIdString, isNonNegativeIdString } from '@/lib/api-types'

import { buildLogSearch } from './search'

const siteIds = z
  .preprocess(
    (value) => {
      if (value == null) return []
      if (Array.isArray(value)) return value
      return [value]
    },
    z.array(z.string().refine(isIdString)).max(100)
  )
  .catch([])

export const logSearchSchema = z
  .object({
    channelId: z
      .string()
      .refine(isNonNegativeIdString)
      .optional()
      .catch(undefined),
    end: z.coerce.number().int().optional().catch(undefined),
    exportId: z.string().refine(isIdString).optional().catch(undefined),
    group: z.string().optional().catch(undefined),
    modelName: z.string().optional().catch(undefined),
    page: z.coerce.number().int().min(1).optional().catch(undefined),
    pageSize: z.coerce
      .number()
      .int()
      .min(1)
      .max(100)
      .optional()
      .catch(undefined),
    requestId: z.string().optional().catch(undefined),
    siteIds,
    start: z.coerce.number().int().optional().catch(undefined),
    tokenName: z.string().optional().catch(undefined),
    type: z.coerce.number().int().min(0).max(7).optional().catch(undefined),
    upstreamRequestId: z.string().optional().catch(undefined),
    username: z.string().optional().catch(undefined),
  })
  .transform((search) => buildLogSearch(search))
