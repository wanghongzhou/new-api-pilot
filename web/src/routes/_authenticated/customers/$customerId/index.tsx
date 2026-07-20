import { createFileRoute } from '@tanstack/react-router'

import { CustomerDetailPage } from '@/features/customers/components/customer-detail-page'
import { customerDetailSearchSchema } from '@/features/customers/schema'

export const Route = createFileRoute('/_authenticated/customers/$customerId/')({
  component: CustomerDetailRoute,
  validateSearch: customerDetailSearchSchema,
})

function CustomerDetailRoute() {
  const { customerId } = Route.useParams()
  const { accountPage } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <CustomerDetailPage
      accountPage={accountPage ?? 1}
      customerId={customerId}
      onAccountPageChange={(page) =>
        void navigate({
          search: (current) => ({ ...current, accountPage: page }),
        })
      }
      onDeleted={() =>
        void navigate({ search: { status: [] }, to: '/customers' })
      }
    />
  )
}
