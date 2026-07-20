import type { CustomerStatus } from './types'

export const customerStatuses = [
  'communicating',
  'signing',
  'using',
  'disabled',
] as const satisfies readonly CustomerStatus[]

export const editableCustomerStatuses = [
  'communicating',
  'signing',
  'using',
] as const satisfies readonly Exclude<CustomerStatus, 'disabled'>[]
