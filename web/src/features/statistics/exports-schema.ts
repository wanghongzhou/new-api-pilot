import { z } from 'zod'

import { isIdString } from '@/lib/api-types'

const optionalExportId = z
  .preprocess(
    (value) => (value === '' || value == null ? undefined : value),
    z.string().refine(isIdString).optional()
  )
  .catch(undefined)

const exportStatuses = [
  'pending',
  'running',
  'success',
  'failed',
  'expired',
] as const

const exportStatusSearch = z
  .preprocess(
    (value) => {
      if (value == null) return []
      return Array.isArray(value) ? value : [value]
    },
    z
      .array(z.enum(exportStatuses))
      .transform((selected) =>
        exportStatuses.filter((status) => selected.includes(status))
      )
  )
  .catch([])

export const exportsSearchSchema = z.object({
  exportId: optionalExportId,
  format: z.enum(['csv', 'xlsx']).optional().catch(undefined),
  order: z.enum(['asc', 'desc']).optional().catch(undefined),
  page: z.coerce.number().int().min(1).optional().catch(undefined),
  pageSize: z.coerce.number().int().min(1).max(100).optional().catch(undefined),
  scope: z
    .enum([
      'global',
      'site',
      'customer',
      'account',
      'model',
      'channel',
      'group',
      'token',
      'node',
      'logs',
      'user_inventory',
      'channel_inventory',
      'performance_history',
      'topup_inventory',
      'redemption_inventory',
      'upstream_tasks',
      'model_catalog',
      'model_rankings',
      'vendor_rankings',
      'subscription_plans',
      'pricing_catalog',
      'group_catalog',
      'system_tasks',
    ])
    .optional()
    .catch(undefined),
  sort: z
    .enum(['created_at', 'finished_at', 'status', 'file_size'])
    .optional()
    .catch(undefined),
  status: exportStatusSearch.optional(),
})
