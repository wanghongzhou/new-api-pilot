import { ArrowLeft01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { OperationsAnalyticsNavigation } from '@/features/operations-analytics/components/operations-analytics-workspace'
import {
  StatisticsPage,
  type StatisticsPageDataSource,
} from '@/features/statistics/components/statistics-page'
import type {
  EntityStatisticsParams,
  StatisticsSearch,
} from '@/features/statistics/types'
import { isIdString, parseIdString } from '@/lib/api-types'

import { getSite, getSiteStatistics } from '../api'
import { siteKeys } from '../query-keys'

function siteStatisticsParams(
  search: StatisticsSearch
): EntityStatisticsParams {
  return {
    end_timestamp: search.end,
    granularity: search.granularity,
    p: search.page,
    page_size: search.pageSize,
    sort_by: search.sort,
    sort_order: search.order,
    start_timestamp: search.start,
  }
}

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
  const parsedSiteId = validSiteId ? parseIdString(siteId) : undefined
  const detailQuery = useQuery({
    enabled: validSiteId,
    queryFn: () => getSite(parseIdString(siteId)),
    queryKey: siteKeys.detail(siteId),
    staleTime: 5 * 60_000,
  })

  if (!parsedSiteId) {
    return (
      <SectionPageLayout
        description={t('site.stats.description')}
        title={t('site.stats.title')}
      >
        <ErrorState title={t('site.detail.invalidId')} />
      </SectionPageLayout>
    )
  }

  const dataSource: StatisticsPageDataSource = {
    query: (nextSearch) =>
      getSiteStatistics(parsedSiteId, siteStatisticsParams(nextSearch)),
    queryKey: (nextSearch) =>
      siteKeys.statistics(siteId, siteStatisticsParams(nextSearch)),
  }

  return (
    <StatisticsPage
      dataSource={dataSource}
      description={t('site.stats.description')}
      entity={{ id: parsedSiteId, scope: 'site' }}
      header={
        <div className='grid gap-4'>
          <DetailBackLink
            render={<Link params={{ siteId }} to='/sites/$siteId' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('site.stats.backToDetail')}
          </DetailBackLink>
          <OperationsAnalyticsNavigation active='statistics' siteId={siteId} />
        </div>
      }
      hideScopeNavigation
      onSearchChange={onSearchChange}
      scope='site'
      search={search}
      title={
        detailQuery.data
          ? t('site.stats.namedTitle', { name: detailQuery.data.name })
          : t('site.stats.title')
      }
    />
  )
}
