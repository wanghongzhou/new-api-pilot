import type { CustomerListItem } from '@/features/customers/types'
import type { SiteListItem } from '@/features/sites/types'

export function isEligibleAccountCustomer(
  customers: readonly CustomerListItem[],
  customerId: string
): boolean {
  return customers.some(
    (customer) => customer.id === customerId && customer.status === 'using'
  )
}

export function isEligibleAccountSite(
  sites: readonly SiteListItem[],
  siteId: string
): boolean {
  return sites.some(
    (site) =>
      site.id === siteId &&
      site.management_status === 'active' &&
      site.auth_status === 'authorized' &&
      site.data_export_enabled === true
  )
}
