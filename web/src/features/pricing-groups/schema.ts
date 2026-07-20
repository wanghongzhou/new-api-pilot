import { z } from 'zod'

import { isIdString } from '@/lib/api-types'

import { buildPricingGroupSearch } from './search'

function arrayValue(value: unknown) {
  if (value == null) return []
  return Array.isArray(value) ? value : [value]
}

export const pricingGroupSearchSchema = z
  .object({
    exportId: z.string().refine(isIdString).optional().catch(undefined),
    group: z.string().optional().catch(undefined),
    keyword: z.string().optional().catch(undefined),
    page: z.coerce.number().int().min(1).optional().catch(undefined),
    pageSize: z.coerce
      .number()
      .int()
      .min(1)
      .max(100)
      .optional()
      .catch(undefined),
    siteIds: z.preprocess(arrayValue, z.array(z.string()).max(100)).catch([]),
    states: z
      .preprocess(arrayValue, z.array(z.enum(['normal', 'missing'])).max(2))
      .catch([]),
    tab: z.enum(['pricing', 'groups']).optional().catch('pricing'),
  })
  .transform(buildPricingGroupSearch)
