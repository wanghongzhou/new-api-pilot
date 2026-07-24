import { ArrowLeft01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { EntityStatistics } from '@/features/statistics/components/entity-statistics'
import type {
  EntityStatisticsParams,
  StatisticsSearch,
} from '@/features/statistics/types'
import { isIdString, parseIdString } from '@/lib/api-types'

import { getSite, getSiteStatistics } from '../api'
import { siteKeys } from '../query-keys'

export function SiteStatsPage({
  onSearchChange,
  search,
  siteId,
}: {
  onSearchChange: (changes: Partial<StatisticsSearch>) => void
  search: StatisticsSearch
  siteId: string
}) {
  const { t } = useTranslation()
  const validSiteId = isIdString(siteId)
  const params: EntityStatisticsParams = {
    end_timestamp: search.end,
    granularity: search.granularity,
    p: search.page,
    page_size: search.pageSize,
    sort_by: search.sort,
    sort_order: search.order,
    start_timestamp: search.start,
  }
  const detailQuery = useQuery({
    enabled: validSiteId,
    queryFn: () => getSite(parseIdString(siteId)),
    queryKey: siteKeys.detail(siteId),
    staleTime: 5 * 60_000,
  })
  const statisticsQuery = useQuery({
    enabled: validSiteId,
    placeholderData: keepPreviousData,
    queryFn: () => getSiteStatistics(parseIdString(siteId), params),
    queryKey: siteKeys.statistics(siteId, params),
    staleTime: 5 * 60_000,
  })
  const response = statisticsQuery.data
  const contractValid =
    response == null ||
    (response.scope === 'site' &&
      response.granularity === search.granularity &&
      response.range.start_timestamp === search.start &&
      response.range.end_timestamp === search.end)
  const rangeTransition = Boolean(
    response &&
    !contractValid &&
    statisticsQuery.isPlaceholderData &&
    statisticsQuery.isFetching
  )
  const data = contractValid || rangeTransition ? response : undefined

  return (
    <SectionPageLayout
      description={t('site.stats.description')}
      title={
        detailQuery.data
          ? t('site.stats.namedTitle', { name: detailQuery.data.name })
          : t('site.stats.title')
      }
    >
      <div className='grid min-w-0 gap-8'>
        <DetailBackLink
          render={<Link params={{ siteId }} to='/sites/$siteId' />}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          {t('site.stats.backToDetail')}
        </DetailBackLink>
        {!validSiteId ? (
          <ErrorState title={t('site.detail.invalidId')} />
        ) : (
          <EntityStatistics
            data={data}
            entityId={parseIdString(siteId)}
            error={
              statisticsQuery.isError ||
              (!contractValid &&
                !rangeTransition &&
                !statisticsQuery.isFetching)
            }
            fetching={statisticsQuery.isFetching}
            loading={statisticsQuery.isPending}
            onRetry={() => void statisticsQuery.refetch()}
            onSearchChange={onSearchChange}
            rangeTransition={rangeTransition}
            scope='site'
            search={search}
          />
        )}
      </div>
    </SectionPageLayout>
  )
}
