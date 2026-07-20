import { z } from 'zod'

import { isIdString } from '@/lib/api-types'

import { buildStatisticsSearch } from './search'

function searchStringArray(schema: z.ZodType<string>) {
  return z
    .preprocess((value) => {
      if (value == null) return []
      return Array.isArray(value) ? value : [value]
    }, z.array(schema).max(100))
    .catch([])
}

const idArraySchema = searchStringArray(
  z.string().regex(/^[1-9]\d*$/, 'Invalid ID')
)
const modelArraySchema = searchStringArray(z.string().min(1).max(255))
const channelArraySchema = searchStringArray(
  z.string().regex(/^[1-9]\d*:(?:0|[1-9]\d*)$/)
)
const groupArraySchema = searchStringArray(z.string().max(128))
const tokenArraySchema = searchStringArray(
  z.string().regex(/^[1-9]\d*:(?:0|[1-9]\d*)$/)
)
const nodeArraySchema = searchStringArray(z.string().max(128))

const rawStatisticsSearchSchema = z.object({
  start: z.coerce.number().int().optional().catch(undefined),
  end: z.coerce.number().int().optional().catch(undefined),
  granularity: z
    .enum(['hour', 'day', 'month', 'year'])
    .optional()
    .catch(undefined),
  metric: z
    .enum(['request_count', 'quota', 'token_used', 'active_users'])
    .optional()
    .catch(undefined),
  display: z.enum(['quota', 'usd', 'cny']).optional().catch(undefined),
  view: z.enum(['chart', 'table']).optional().catch(undefined),
  page: z.coerce.number().int().min(1).optional().catch(undefined),
  pageSize: z.coerce.number().int().min(1).max(100).optional().catch(undefined),
  sort: z
    .enum([
      'request_count',
      'quota',
      'token_used',
      'active_users',
      'name',
      'bucket_start',
    ])
    .optional()
    .catch(undefined),
  order: z.enum(['asc', 'desc']).optional().catch(undefined),
  siteIds: idArraySchema,
  customerIds: idArraySchema,
  accountIds: idArraySchema,
  models: modelArraySchema,
  channelKeys: channelArraySchema,
  useGroups: groupArraySchema,
  tokenKeys: tokenArraySchema,
  nodeNames: nodeArraySchema,
  exportId: z.string().refine(isIdString).optional().catch(undefined),
})

export const statisticsSearchSchema = rawStatisticsSearchSchema.transform(
  (search) => buildStatisticsSearch(search)
)
