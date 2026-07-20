import { z } from 'zod'

import { isDecimalString, isIdString, isMetricString } from '@/lib/api-types'

import { buildChannelInventorySearch } from './search'

function arrayValue(value: unknown) {
  if (value == null) return []
  return Array.isArray(value) ? value : [value]
}

const numberArray = (maximum: number) =>
  z
    .preprocess(
      arrayValue,
      z.array(z.coerce.number().int().min(0).max(maximum))
    )
    .catch([])
const stringArray = (schema: z.ZodType<string>, maximum = 100) =>
  z.preprocess(arrayValue, z.array(schema).max(maximum)).catch([])

export const channelInventorySearchSchema = z
  .object({
    end: z.coerce.number().int().optional().catch(undefined),
    exportId: z.string().refine(isIdString).optional().catch(undefined),
    groups: stringArray(z.string().max(128)),
    keyword: z.string().optional().catch(undefined),
    maxBalance: z.string().refine(isDecimalString).optional().catch(undefined),
    maxResponseTime: z
      .string()
      .refine((value) => isMetricString(value) && !value.startsWith('-'))
      .optional()
      .catch(undefined),
    minBalance: z.string().refine(isDecimalString).optional().catch(undefined),
    minResponseTime: z
      .string()
      .refine((value) => isMetricString(value) && !value.startsWith('-'))
      .optional()
      .catch(undefined),
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
    states: stringArray(z.enum(['normal', 'missing']), 2),
    statuses: numberArray(3),
    tags: stringArray(z.string().max(128)),
    types: numberArray(10_000),
  })
  .transform((search) => buildChannelInventorySearch(search))
