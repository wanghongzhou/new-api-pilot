import { createFileRoute } from '@tanstack/react-router'

import { SiteInstancesPage } from '@/features/sites/components/site-instances-page'
import { siteStatusSearchSchema } from '@/features/sites/schema'
import { dayjs } from '@/lib/dayjs'

export const Route = createFileRoute('/_authenticated/sites/$siteId/status')({
  component: SiteStatusRoute,
  validateSearch: siteStatusSearchSchema,
})

function SiteStatusRoute() {
  const { siteId } = Route.useParams()
  const rawSearch = Route.useSearch()
  const navigate = Route.useNavigate()
  const granularity = rawSearch.granularity ?? 'minute'
  const metric = rawSearch.metric ?? 'cpu'
  const defaultEnd = dayjs().startOf('minute').unix()
  let defaultDuration = 30 * 24 * 60 * 60
  if (granularity === 'minute') defaultDuration = 24 * 60 * 60
  else if (granularity === 'hour') defaultDuration = 7 * 24 * 60 * 60
  let aggregation = rawSearch.aggregation ?? 'max'
  if (metric === 'disk' && aggregation === 'avg') aggregation = 'last'
  if (metric !== 'disk' && aggregation === 'last') aggregation = 'max'

  return (
    <SiteInstancesPage
      onSearchChange={(changes) =>
        void navigate({
          search: (current) => ({ ...current, ...changes }),
        })
      }
      search={{
        aggregation,
        end: rawSearch.end ?? defaultEnd,
        granularity,
        metric,
        nodeName: rawSearch.nodeName,
        start: rawSearch.start ?? defaultEnd - defaultDuration,
      }}
      siteId={siteId}
    />
  )
}
