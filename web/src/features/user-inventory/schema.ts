import { z } from 'zod'

import { isIdString, isMetricString } from '@/lib/api-types'

import { buildUserInventorySearch } from './search'

function arrayValue(value: unknown) {
  if (value == null) return []
  return Array.isArray(value) ? value : [value]
}

const numberArray = (values: readonly number[]) =>
  z
    .preprocess(arrayValue, z.array(z.coerce.number().int()).max(20))
    .transform((selected) => selected.filter((value) => values.includes(value)))
    .catch([])

const stringArray = (schema: z.ZodType<string>, maximum = 100) =>
  z.preprocess(arrayValue, z.array(schema).max(maximum)).catch([])

export const userInventorySearchSchema = z
  .object({
    end: z.coerce.number().int().optional().catch(undefined),
    exportId: z.string().refine(isIdString).optional().catch(undefined),
    groups: stringArray(z.string().max(128)),
    keyword: z.string().optional().catch(undefined),
    maxBalance: z.string().refine(isMetricString).optional().catch(undefined),
    minBalance: z.string().refine(isMetricString).optional().catch(undefined),
    page: z.coerce.number().int().min(1).optional().catch(undefined),
    pageSize: z.coerce
      .number()
      .int()
      .min(1)
      .max(100)
      .optional()
      .catch(undefined),
    roles: numberArray([0, 1, 10, 100]),
    remoteUserId: z.string().refine(isIdString).optional().catch(undefined),
    siteIds: stringArray(z.string().refine(isIdString)),
    start: z.coerce.number().int().optional().catch(undefined),
    states: stringArray(
      z.enum(['normal', 'missing', 'deleted', 'identity_mismatch']),
      4
    ),
    statuses: numberArray([1, 2]),
  })
  .transform((search) => buildUserInventorySearch(search))
