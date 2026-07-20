import { createFileRoute } from '@tanstack/react-router'

import { AccountDetailPage } from '@/features/accounts/components/account-detail-page'

export const Route = createFileRoute('/_authenticated/accounts/$accountId/')({
  component: AccountDetailRoute,
})

function AccountDetailRoute() {
  const { accountId } = Route.useParams()
  const navigate = Route.useNavigate()
  return (
    <AccountDetailPage
      accountId={accountId}
      onDeleted={() =>
        void navigate({
          search: { managedStatus: [], remoteState: [], remoteStatus: [] },
          to: '/accounts',
        })
      }
    />
  )
}
