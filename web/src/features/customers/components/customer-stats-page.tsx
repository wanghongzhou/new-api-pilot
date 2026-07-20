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

import { getCustomer, getCustomerStatistics } from '../api'
import { customerKeys } from '../query-keys'

export function CustomerStatsPage({
  customerId,
  onSearchChange,
  search,
}: {
  customerId: string
  onSearchChange: (changes: Partial<StatisticsSearch>) => void
  search: StatisticsSearch
}) {
  const { t } = useTranslation()
  const validCustomerId = isIdString(customerId)
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
    enabled: validCustomerId,
    queryFn: () => getCustomer(parseIdString(customerId)),
    queryKey: customerKeys.detail(customerId),
    staleTime: 5 * 60_000,
  })
  const statisticsQuery = useQuery({
    enabled: validCustomerId,
    placeholderData: keepPreviousData,
    queryFn: () => getCustomerStatistics(parseIdString(customerId), params),
    queryKey: customerKeys.statistics(customerId, params),
    staleTime: 5 * 60_000,
  })
  const response = statisticsQuery.data
  const contractValid =
    response == null ||
    (response.scope === 'customer' &&
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
      description={t('customer.stats.description')}
      title={
        detailQuery.data
          ? t('customer.stats.namedTitle', { name: detailQuery.data.name })
          : t('customer.stats.title')
      }
    >
      <div className='grid min-w-0 gap-8'>
        <DetailBackLink
          render={<Link params={{ customerId }} to='/customers/$customerId' />}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          {t('customer.stats.backToDetail')}
        </DetailBackLink>
        {!validCustomerId ? (
          <section className='border-destructive/30 bg-destructive/5 rounded-lg border p-5'>
            <h2 className='font-medium'>{t('customer.detail.invalidId')}</h2>
          </section>
        ) : (
          <EntityStatistics
            data={data}
            entityId={parseIdString(customerId)}
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
            scope='customer'
            rangeTransition={rangeTransition}
          />
        )}
      </div>
    </SectionPageLayout>
  )
}
