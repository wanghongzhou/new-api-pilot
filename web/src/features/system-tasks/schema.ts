import { z } from 'zod'

import { isIdString } from '@/lib/api-types'

import { buildSystemTaskSearch } from './search'
import { systemTaskStatuses, systemTaskTypes } from './types'

function arrayValue(value: unknown) {
  if (value == null) return []
  return Array.isArray(value) ? value : [value]
}
const booleanValue = z.preprocess((value) => {
  if (value === 'true') return true
  if (value === 'false') return false
  return value
}, z.boolean().optional())

export const systemTaskSearchSchema = z
  .object({
    createdEnd: z.coerce.number().int().positive().optional().catch(undefined),
    createdStart: z.coerce
      .number()
      .int()
      .positive()
      .optional()
      .catch(undefined),
    errorPresent: booleanValue.catch(undefined),
    exportId: z.string().refine(isIdString).optional().catch(undefined),
    page: z.coerce.number().int().min(1).optional().catch(undefined),
    pageSize: z.coerce
      .number()
      .int()
      .min(1)
      .max(100)
      .optional()
      .catch(undefined),
    siteIds: z.preprocess(arrayValue, z.array(z.string()).max(100)).catch([]),
    statuses: z
      .preprocess(arrayValue, z.array(z.enum(systemTaskStatuses)).max(4))
      .catch([]),
    types: z
      .preprocess(arrayValue, z.array(z.enum(systemTaskTypes)).max(5))
      .catch([]),
  })
  .transform(buildSystemTaskSearch)
