import { createFileRoute } from '@tanstack/react-router'

import { AccountsPage } from '@/features/accounts/components/accounts-page'
import { accountsSearchSchema } from '@/features/accounts/schema'
import type { AccountSearch } from '@/features/accounts/types'
import { isIdString } from '@/lib/api-types'

export const Route = createFileRoute('/_authenticated/accounts/')({
  component: AccountsRoute,
  validateSearch: accountsSearchSchema,
})

function AccountsRoute() {
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  const search: AccountSearch = {
    customerId: isIdString(rawSearch.customer_id)
      ? rawSearch.customer_id
      : undefined,
    filter: rawSearch.filter ?? '',
    managedStatus: rawSearch.managedStatus,
    order: rawSearch.order ?? 'desc',
    page: rawSearch.page ?? 1,
    pageSize: rawSearch.pageSize ?? 20,
    remoteState: rawSearch.remoteState,
    remoteStatus: rawSearch.remoteStatus,
    siteId: isIdString(rawSearch.site_id) ? rawSearch.site_id : undefined,
    sort: rawSearch.sort ?? 'updated_at',
  }
  return (
    <AccountsPage
      onOpenAccount={(accountId) =>
        void navigate({ params: { accountId }, to: '/accounts/$accountId' })
      }
      onSearchChange={(changes) => {
        const hasCustomerId = Object.hasOwn(changes, 'customerId')
        const hasSiteId = Object.hasOwn(changes, 'siteId')
        void navigate({
          search: (current) => ({
            ...current,
            customer_id: hasCustomerId
              ? changes.customerId
              : current.customer_id,
            filter: changes.filter ?? current.filter,
            managedStatus: changes.managedStatus ?? current.managedStatus,
            order: changes.order ?? current.order,
            page: changes.page ?? current.page,
            pageSize: changes.pageSize ?? current.pageSize,
            remoteState: changes.remoteState ?? current.remoteState,
            remoteStatus: changes.remoteStatus ?? current.remoteStatus,
            site_id: hasSiteId ? changes.siteId : current.site_id,
            sort: changes.sort ?? current.sort,
          }),
        })
      }}
      search={search}
    />
  )
}
