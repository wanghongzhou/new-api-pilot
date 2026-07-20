import { createFileRoute } from '@tanstack/react-router'

import { CustomersPage } from '@/features/customers/components/customers-page'
import { customersSearchSchema } from '@/features/customers/schema'
import type { CustomerSearch } from '@/features/customers/types'

export const Route = createFileRoute('/_authenticated/customers/')({
  component: CustomersRoute,
  validateSearch: customersSearchSchema,
})

function CustomersRoute() {
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  const storedView = window.localStorage.getItem('customers:view-mode')
  const preferredView: CustomerSearch['view'] =
    storedView === 'table' || storedView === 'card' ? storedView : 'card'
  const search: CustomerSearch = {
    filter: rawSearch.filter ?? '',
    order: rawSearch.order ?? 'desc',
    page: rawSearch.page ?? 1,
    pageSize: rawSearch.pageSize ?? 20,
    sort: rawSearch.sort ?? 'updated_at',
    status: rawSearch.status,
    view: rawSearch.view ?? preferredView,
  }

  return (
    <CustomersPage
      onOpenAccounts={(customerId) =>
        void navigate({
          search: {
            customer_id: customerId,
            managedStatus: [],
            remoteState: [],
            remoteStatus: [],
          },
          to: '/accounts',
        })
      }
      onSearchChange={(changes) => {
        if (changes.view) {
          window.localStorage.setItem('customers:view-mode', changes.view)
        }
        void navigate({
          search: (current) => ({ ...current, ...changes }),
        })
      }}
      search={search}
    />
  )
}
