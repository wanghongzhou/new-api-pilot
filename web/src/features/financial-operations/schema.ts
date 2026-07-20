import { z } from 'zod'

import { isIdString, isNonNegativeIdString } from '@/lib/api-types'

import { buildFinancialOperationsSearch } from './search'

function arrayValue(value: unknown) {
  if (value == null) return []
  return Array.isArray(value) ? value : [value]
}

const strings = (schema: z.ZodType<string>) =>
  z.preprocess(arrayValue, z.array(schema).max(100)).catch([])

export const financialOperationsSearchSchema = z
  .object({
    end: z.coerce.number().int().optional().catch(undefined),
    exportId: z.string().refine(isIdString).optional().catch(undefined),
    keyword: z.string().max(255).optional().catch(undefined),
    methods: strings(z.string().max(255)),
    page: z.coerce.number().int().min(1).optional().catch(undefined),
    pageSize: z.coerce
      .number()
      .int()
      .min(1)
      .max(100)
      .optional()
      .catch(undefined),
    providers: strings(z.string().max(255)),
    remoteId: z.string().refine(isIdString).optional().catch(undefined),
    remoteUserId: z
      .string()
      .refine(isNonNegativeIdString)
      .optional()
      .catch(undefined),
    siteIds: strings(z.string().refine(isIdString)),
    start: z.coerce.number().int().optional().catch(undefined),
    states: strings(z.enum(['normal', 'missing'])),
    statuses: strings(z.string().max(255)),
    tab: z.enum(['topups', 'redemptions']).optional().catch(undefined),
  })
  .transform((search) => buildFinancialOperationsSearch(search))
