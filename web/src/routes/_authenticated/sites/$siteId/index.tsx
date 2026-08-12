import { createFileRoute } from '@tanstack/react-router'

import { SiteDetailPage } from '@/features/sites/components/site-detail-page'

export const Route = createFileRoute('/_authenticated/sites/$siteId/')({
  component: SiteDetailRoute,
})

function SiteDetailRoute() {
  const { siteId } = Route.useParams()
  const navigate = Route.useNavigate()
  return (
    <SiteDetailPage
      onDeleted={() =>
        void navigate({
          search: {
            auth: [],
            health: [],
            management: [],
            online: [],
            statistics: [],
          },
          to: '/sites',
        })
      }
      siteId={siteId}
    />
  )
}
