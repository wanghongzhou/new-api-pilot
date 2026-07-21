import { z } from 'zod'

import { customerStatuses, editableCustomerStatuses } from './constants'

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

export const customersSearchSchema = z.object({
  page: z.coerce.number().int().min(1).optional().catch(undefined),
  pageSize: z.coerce.number().int().min(1).max(100).optional().catch(undefined),
  filter: z.string().max(128).optional().catch(undefined),
  status: searchArray(customerStatuses),
  view: z.enum(['card', 'table']).optional().catch(undefined),
  sort: z
    .enum(['updated_at', 'name', 'today_quota', 'account_count'])
    .optional()
    .catch(undefined),
  order: z.enum(['asc', 'desc']).optional().catch(undefined),
})

export const customerDetailSearchSchema = z.object({
  accountPage: z.coerce.number().int().min(1).optional().catch(undefined),
})

export const customerFormSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'customer.validation.nameRequired')
    .max(128, 'customer.validation.nameLength'),
  contact: z.string().trim().max(255, 'customer.validation.contactLength'),
  remark: z.string().trim().max(500, 'customer.validation.remarkLength'),
  contract_amount: z
    .string()
    .trim()
    .regex(
      /^$|^(0|[1-9][0-9]*)(\.[0-9]{1,10})?$/,
      'customer.validation.amount'
    ),
  payment_amount: z
    .string()
    .trim()
    .regex(
      /^$|^(0|[1-9][0-9]*)(\.[0-9]{1,10})?$/,
      'customer.validation.amount'
    ),
  status: z.enum(editableCustomerStatuses),
})

export type CustomerFormValues = z.input<typeof customerFormSchema>
export type CustomerFormOutput = z.output<typeof customerFormSchema>
