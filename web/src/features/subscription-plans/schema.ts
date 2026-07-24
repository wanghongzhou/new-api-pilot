import { z } from 'zod'

import { isIdString } from '@/lib/api-types'

function optionalArrayValue(value: unknown) {
  if (value == null) return undefined
  return Array.isArray(value) ? value : [value]
}

const enabledValue = z.preprocess((value) => {
  if (value === 'true') return true
  if (value === 'false') return false
  return value
}, z.boolean().optional())

export const subscriptionPlanSearchSchema = z.object({
  enabled: enabledValue.catch(undefined),
  exportId: z.string().refine(isIdString).optional().catch(undefined),
  keyword: z.string().optional().catch(undefined),
  page: z.coerce.number().int().min(1).optional().catch(undefined),
  pageSize: z.coerce.number().int().min(1).max(100).optional().catch(undefined),
  siteIds: z
    .preprocess(optionalArrayValue, z.array(z.string()).max(100).optional())
    .catch(undefined),
  states: z
    .preprocess(
      optionalArrayValue,
      z
        .array(z.enum(['normal', 'missing']))
        .max(2)
        .optional()
    )
    .catch(undefined),
  tab: z.enum(['plans', 'site-analysis']).optional().catch(undefined),
})
