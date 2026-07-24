import { z } from 'zod'

import { isIdString, isNonNegativeIdString } from '@/lib/api-types'

function optionalArrayValue(value: unknown) {
  if (value == null) return undefined
  return Array.isArray(value) ? value : [value]
}

const binaryArray = z
  .preprocess(
    optionalArrayValue,
    z.array(z.coerce.number().int()).max(2).optional()
  )
  .catch(undefined)

export const modelCatalogSearchSchema = z.object({
  exportId: z.string().refine(isIdString).optional().catch(undefined),
  keyword: z.string().optional().catch(undefined),
  page: z.coerce.number().int().min(1).optional().catch(undefined),
  pageSize: z.coerce.number().int().min(1).max(100).optional().catch(undefined),
  siteIds: z
    .preprocess(optionalArrayValue, z.array(z.string()).max(100).optional())
    .catch(undefined),
  statuses: binaryArray,
  syncOfficial: binaryArray,
  tab: z.enum(['catalog', 'coverage', 'missing']).optional().catch(undefined),
  vendorId: z
    .string()
    .refine(isNonNegativeIdString)
    .optional()
    .catch(undefined),
})
