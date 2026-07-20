import { z } from 'zod'

import {
  accountManagedStatuses,
  accountRemoteStates,
  remoteUserStatusFilters,
} from './constants'

const idSchema = z.string().regex(/^[1-9]\d*$/, 'common.validation.invalidId')

function searchArray<T extends readonly [string, ...string[]]>(values: T) {
  return z
    .preprocess(
      (value) => {
        if (value == null) return []
        return Array.isArray(value) ? value : [value]
      },
      z.array(z.enum(values))
    )
    .catch([])
}

export const accountsSearchSchema = z.object({
  page: z.coerce.number().int().min(1).optional().catch(undefined),
  pageSize: z.coerce.number().int().min(1).max(100).optional().catch(undefined),
  filter: z.string().max(128).optional().catch(undefined),
  site_id: z.coerce
    .string()
    .regex(/^[1-9]\d*$/)
    .optional()
    .catch(undefined),
  customer_id: z.coerce
    .string()
    .regex(/^[1-9]\d*$/)
    .optional()
    .catch(undefined),
  remoteStatus: searchArray(remoteUserStatusFilters),
  remoteState: searchArray(accountRemoteStates),
  managedStatus: searchArray(accountManagedStatuses),
  sort: z
    .enum(['updated_at', 'username', 'today_quota', 'quota'])
    .optional()
    .catch(undefined),
  order: z.enum(['asc', 'desc']).optional().catch(undefined),
})

export const accountOnboardingSchema = z.object({
  customerId: idSchema,
  siteId: idSchema,
  remoteUserId: idSchema,
  remark: z.string().trim().max(500, 'account.validation.remarkLength'),
  bindingConfirmed: z.boolean().refine((confirmed) => confirmed, {
    error: 'account.validation.bindingConfirmationRequired',
  }),
})

export const accountCustomerStepSchema = accountOnboardingSchema.pick({
  customerId: true,
})
export const accountRemoteUserStepSchema = accountOnboardingSchema.pick({
  remoteUserId: true,
  siteId: true,
})

export const accountRemarkSchema = z.object({
  remark: z.string().trim().max(500, 'account.validation.remarkLength'),
})

export type AccountOnboardingValues = z.input<typeof accountOnboardingSchema>
export type AccountRemarkValues = z.input<typeof accountRemarkSchema>
