import { z } from 'zod'

import { isIdString } from '@/lib/api-types'

import { buildRankingSearch } from './search'

function arrayValue(value: unknown) {
  if (value == null) return []
  return Array.isArray(value) ? value : [value]
}

export const rankingSearchSchema = z
  .object({
    exportId: z.string().refine(isIdString).optional().catch(undefined),
    period: z
      .enum(['today', 'week', 'month', 'year'])
      .optional()
      .catch(undefined),
    siteIds: z.preprocess(arrayValue, z.array(z.string()).max(100)).catch([]),
    tab: z.enum(['models', 'vendors']).optional().catch(undefined),
  })
  .transform(buildRankingSearch)
