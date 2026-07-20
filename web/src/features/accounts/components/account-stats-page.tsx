import { ArrowLeft01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { EntityStatistics } from '@/features/statistics/components/entity-statistics'
import type {
  EntityStatisticsParams,
  StatisticsSearch,
} from '@/features/statistics/types'
import { isIdString, parseIdString } from '@/lib/api-types'

import { getAccount, getAccountStatistics } from '../api'
import { accountKeys } from '../query-keys'

export function AccountStatsPage({
  accountId,
  onSearchChange,
  search,
}: {
  accountId: string
  onSearchChange: (changes: Partial<StatisticsSearch>) => void
  search: StatisticsSearch
}) {
  const { t } = useTranslation()
  const validAccountId = isIdString(accountId)
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
    enabled: validAccountId,
    queryFn: () => getAccount(parseIdString(accountId)),
    queryKey: accountKeys.detail(accountId),
    staleTime: 5 * 60_000,
  })
  const statisticsQuery = useQuery({
    enabled: validAccountId,
    placeholderData: keepPreviousData,
    queryFn: () => getAccountStatistics(parseIdString(accountId), params),
    queryKey: accountKeys.statistics(accountId, params),
    staleTime: 5 * 60_000,
  })
  const response = statisticsQuery.data
  const contractValid =
    response == null ||
    (response.scope === 'account' &&
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
      description={t('account.stats.description')}
      title={
        detailQuery.data
          ? t('account.stats.namedTitle', { name: detailQuery.data.username })
          : t('account.stats.title')
      }
    >
      <div className='grid min-w-0 gap-8'>
        <DetailBackLink
          render={<Link params={{ accountId }} to='/accounts/$accountId' />}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          {t('account.stats.backToDetail')}
        </DetailBackLink>
        {!validAccountId ? (
          <section className='border-destructive/30 bg-destructive/5 rounded-lg border p-5'>
            <h2 className='font-medium'>{t('account.detail.invalidId')}</h2>
          </section>
        ) : (
          <EntityStatistics
            data={data}
            entityId={parseIdString(accountId)}
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
            search={search}
            scope='account'
            rangeTransition={rangeTransition}
          />
        )}
      </div>
    </SectionPageLayout>
  )
}
