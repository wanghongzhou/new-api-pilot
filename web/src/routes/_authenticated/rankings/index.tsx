import { createFileRoute } from '@tanstack/react-router'

import { RankingsPage } from '@/features/rankings/components/rankings-page'
import { rankingSearchSchema } from '@/features/rankings/schema'
import { buildRankingSearch } from '@/features/rankings/search'
export const Route = createFileRoute('/_authenticated/rankings/')({
  component: GlobalRankings,
  validateSearch: rankingSearchSchema,
})
function GlobalRankings() {
  const search = buildRankingSearch(Route.useSearch())
  const navigate = Route.useNavigate()
  return (
    <RankingsPage
      search={search}
      onSearchChange={(changes) =>
        void navigate({ search: (current) => ({ ...current, ...changes }) })
      }
    />
  )
}
