import { z } from 'zod'

import { isIdString } from '@/lib/api-types'

import { buildPerformanceHistorySearch } from './search'

function arrayValue(value: unknown) {
  if (value == null) return []
  return Array.isArray(value) ? value : [value]
}

const stringArray = (schema: z.ZodType<string>) =>
  z.preprocess(arrayValue, z.array(schema).max(100)).catch([])

export const performanceHistorySearchSchema = z
  .object({
    end: z.coerce.number().int().optional().catch(undefined),
    exportId: z.string().refine(isIdString).optional().catch(undefined),
    groups: stringArray(z.string().max(255)),
    hours: z.coerce
      .number()
      .pipe(z.union([z.literal(24), z.literal(168), z.literal(720)]))
      .optional()
      .catch(undefined),
    models: stringArray(z.string().max(255)),
    page: z.coerce.number().int().min(1).optional().catch(undefined),
    pageSize: z.coerce
      .number()
      .int()
      .min(1)
      .max(100)
      .optional()
      .catch(undefined),
    siteIds: stringArray(z.string().refine(isIdString)),
    start: z.coerce.number().int().optional().catch(undefined),
  })
  .transform((search) => buildPerformanceHistorySearch(search))
