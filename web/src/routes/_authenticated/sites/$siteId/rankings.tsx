import { createFileRoute } from '@tanstack/react-router'

import { RankingsPage } from '@/features/rankings/components/rankings-page'
import { rankingSearchSchema } from '@/features/rankings/schema'
import { buildRankingSearch } from '@/features/rankings/search'

export const Route = createFileRoute('/_authenticated/sites/$siteId/rankings')({
  component: SiteRankings,
  validateSearch: rankingSearchSchema,
})
function SiteRankings() {
  const { siteId } = Route.useParams()
  const search = buildRankingSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <RankingsPage
      search={search}
      siteId={siteId}
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
    />
  )
}
