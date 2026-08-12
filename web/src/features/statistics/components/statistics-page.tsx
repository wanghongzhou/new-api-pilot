import { Chart01Icon } from '@hugeicons/core-free-icons'
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CompletenessAlert } from '@/components/data/completeness-alert'
import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { DataTable } from '@/components/ui/data-table'
import { Spinner } from '@/components/ui/spinner'
import { OperationsViewPurpose } from '@/features/operations-analytics/components/operations-analytics-workspace'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import type { IdString } from '@/lib/api-types'
import { formatBeijingTimestamp } from '@/lib/dayjs'
import { formatDisplayValue } from '@/lib/display-value'
import { cn } from '@/lib/utils'

import { createStatisticsExport, getStatistics } from '../api'
import {
  buildEntityExportRequest,
  buildStatisticsExportRequest,
} from '../export-request'
import { statisticsKeys } from '../query-keys'
import { buildScopeStatisticsSearch } from '../search'
import type {
  AccountStatisticsBreakdown,
  ChannelStatisticsBreakdown,
  CustomerStatisticsBreakdown,
  GroupStatisticsBreakdown,
  GlobalStatisticsBreakdown,
  ModelStatisticsBreakdown,
  NodeStatisticsBreakdown,
  SiteStatisticsBreakdown,
  StatisticsBreakdownBase,
  StatisticsExportFormat,
  StatisticsExportJobItem,
  StatisticsQueryParams,
  StatisticsResponse,
  StatisticsScope,
  StatisticsSearch,
  TokenStatisticsBreakdown,
} from '../types'
import {
  ActiveUsersValue,
  AmountValue,
  MetricTrendChart,
  SiteBreakdownDisclosure,
  StatisticsSummary,
  StatisticsToolbar,
  useStatisticsEmptyCopy,
} from './entity-statistics'
import { ExportDialog } from './export-dialog'
import { ExportTaskSheet } from './export-task-sheet'
import { StatisticsFilters } from './statistics-filters'

const scopeLinks = [
  ['global', '/statistics/global'],
  ['site', '/statistics/sites'],
  ['customer', '/statistics/customers'],
  ['account', '/statistics/accounts'],
  ['model', '/statistics/models'],
  ['channel', '/statistics/channels'],
  ['group', '/statistics/groups'],
  ['token', '/statistics/tokens'],
  ['node', '/statistics/nodes'],
] as const

export interface StatisticsPageDataSource {
  query: (search: StatisticsSearch) => Promise<StatisticsResponse>
  queryKey: (search: StatisticsSearch) => readonly unknown[]
}

function queryParams(search: StatisticsSearch): StatisticsQueryParams {
  return {
    account_ids: search.accountIds,
    channel_keys: search.channelKeys,
    customer_ids: search.customerIds,
    end_timestamp: search.end,
    granularity: search.granularity,
    model_names: search.models,
    node_names: search.nodeNames,
    p: search.page,
    page_size: search.pageSize,
    site_ids: search.siteIds,
    sort_by: search.sort,
    sort_order: search.order,
    start_timestamp: search.start,
    token_keys: search.tokenKeys,
    use_groups: search.useGroups,
  }
}

function ScopeNavigation({
  scope,
  search,
}: {
  scope: StatisticsScope
  search: StatisticsSearch
}) {
  const { t } = useTranslation()
  return (
    <nav
      aria-label={t('statistics.scopeNavigation')}
      className='bg-muted text-muted-foreground flex w-fit max-w-full flex-wrap items-center justify-start gap-0.5 rounded-lg p-[3px]'
    >
      {scopeLinks.map(([value, to]) => {
        const active = value === scope
        return (
          <Link
            aria-current={active ? 'page' : undefined}
            className={cn(
              'text-foreground/60 hover:text-foreground focus-visible:border-ring focus-visible:ring-ring/50 inline-flex min-h-10 items-center justify-center rounded-md border border-transparent px-2 py-1 text-sm font-medium whitespace-nowrap transition-all focus-visible:ring-[3px] focus-visible:outline-1 sm:min-h-7',
              active &&
                'bg-background text-foreground shadow-sm dark:border-input dark:bg-input/30'
            )}
            key={value}
            search={buildScopeStatisticsSearch(search, value)}
            to={to}
          >
            {t(dynamicI18nKey('statistics', `statistics.scope.${value}`))}
          </Link>
        )
      })}
    </nav>
  )
}

function ScopeDetails({
  item,
  scope,
}: {
  item: StatisticsBreakdownBase
  scope: StatisticsScope
}) {
  const { t } = useTranslation()
  if (scope === 'global') {
    const value = item as GlobalStatisticsBreakdown
    return (
      <span>
        {t('statistics.scopeDetails.coverage', {
          complete: value.complete_site_count,
          expected: value.expected_site_count,
        })}
      </span>
    )
  }
  if (scope === 'site') {
    const value = item as SiteStatisticsBreakdown
    return (
      <div className='flex max-w-72 flex-wrap gap-1'>
        <Badge variant='neutral'>
          {t(
            dynamicI18nKey('site', `site.management.${value.management_status}`)
          )}
        </Badge>
        <Badge variant='neutral'>
          {t(dynamicI18nKey('site', `site.online.${value.online_status}`))}
        </Badge>
        <Badge variant='neutral'>
          {t(dynamicI18nKey('site', `site.auth.${value.auth_status}`))}
        </Badge>
        <Badge variant='neutral'>
          {t(
            dynamicI18nKey('site', `site.statistics.${value.statistics_status}`)
          )}
        </Badge>
      </div>
    )
  }
  if (scope === 'customer') {
    const value = item as CustomerStatisticsBreakdown
    return (
      <span>
        {t('statistics.scopeDetails.customer', {
          accounts: value.account_count,
          sites: value.site_count,
        })}
      </span>
    )
  }
  if (scope === 'account') {
    const value = item as AccountStatisticsBreakdown
    return (
      <div className='grid gap-1 text-xs'>
        <span>
          {t('statistics.identity.named', {
            id: value.site_id,
            name: value.site_name,
          })}
        </span>
        <span>
          {t('statistics.identity.named', {
            id: value.customer_id,
            name: value.customer_name,
          })}
        </span>
        <code className='break-all'>{value.remote_user_id}</code>
      </div>
    )
  }
  if (scope === 'model') {
    const value = item as ModelStatisticsBreakdown
    return value.site_name ? (
      <span>{value.site_name}</span>
    ) : (
      <span className='text-muted-foreground'>
        {formatDisplayValue(value.site_name)}
      </span>
    )
  }
  if (scope === 'channel') {
    const value = item as ChannelStatisticsBreakdown
    return (
      <div className='grid gap-1 text-xs'>
        <span>{formatDisplayValue(value.site_name)}</span>
        <code>{value.remote_channel_id}</code>
        {value.remote_missing && (
          <Badge variant='warning'>
            {t('statistics.channel.remoteMissing')}
          </Badge>
        )}
      </div>
    )
  }
  if (scope === 'group') {
    const value = item as GroupStatisticsBreakdown
    return (
      <div className='grid gap-1 text-xs'>
        <span>{formatDisplayValue(value.site_name)}</span>
        <code className='break-all'>
          {value.use_group || t('statistics.group.unknown')}
        </code>
      </div>
    )
  }
  if (scope === 'token') {
    const value = item as TokenStatisticsBreakdown
    return (
      <div className='grid gap-1 text-xs'>
        <span>{formatDisplayValue(value.site_name)}</span>
        <span>
          {value.token_name ||
            (value.token_id === '0'
              ? t('statistics.token.unknownDeleted')
              : t('statistics.token.unnamed'))}
        </span>
        <code className='break-all'>
          {t('statistics.token.id', { id: value.token_id })}
        </code>
      </div>
    )
  }
  const value = item as NodeStatisticsBreakdown
  return (
    <div className='grid gap-1 text-xs'>
      <span>{formatDisplayValue(value.site_name)}</span>
      <code className='break-all'>
        {value.node_name || t('statistics.node.unknown')}
      </code>
    </div>
  )
}

function dimensionLabel(
  item: StatisticsBreakdownBase,
  scope: StatisticsScope,
  translate: ReturnType<typeof useTranslation>['t']
) {
  if (scope === 'group') {
    return (
      (item as GroupStatisticsBreakdown).use_group ||
      translate('statistics.group.unknown')
    )
  }
  if (scope === 'token') {
    const token = item as TokenStatisticsBreakdown
    return (
      token.token_name ||
      (token.token_id === '0'
        ? translate('statistics.token.unknownDeleted')
        : translate('statistics.token.unnamed'))
    )
  }
  if (scope === 'node') {
    return (
      (item as NodeStatisticsBreakdown).node_name ||
      translate('statistics.node.unknown')
    )
  }
  return item.dimension_name || translate('common.none')
}

function BreakdownMobileCard({
  item,
  scope,
  search,
}: {
  item: StatisticsBreakdownBase
  scope: StatisticsScope
  search: StatisticsSearch
}) {
  const { t } = useTranslation()
  return (
    <article className='bg-card text-card-foreground ring-foreground/10 grid min-w-0 gap-4 rounded-xl p-4 ring-1'>
      <header className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
        <div className='min-w-0'>
          <h3 className='font-medium break-words'>
            {dimensionLabel(item, scope, t)}
          </h3>
          <p className='text-muted-foreground mt-1 text-xs break-all'>
            {t('statistics.identity.bucket', {
              id: item.dimension_id,
              time: formatBeijingTimestamp(
                item.bucket_start,
                search.granularity
              ),
            })}
          </p>
        </div>
        <DataStatusBadge status={item.data_status} />
      </header>
      <ScopeDetails item={item} scope={scope} />
      <div className='grid grid-cols-2 gap-3 text-sm'>
        <dl>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.request_count')}
          </dt>
          <dd>
            <MetricValue compact value={item.request_count} />
          </dd>
        </dl>
        <dl>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.quota')}
          </dt>
          <dd>
            <MetricValue compact value={item.quota} />
          </dd>
        </dl>
        <dl>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.token_used')}
          </dt>
          <dd>
            <MetricValue compact value={item.token_used} />
          </dd>
        </dl>
        <dl>
          <dt className='text-muted-foreground text-xs'>
            {t('statistics.metric.active_users')}
          </dt>
          <dd>
            <ActiveUsersValue compact scope={scope} value={item.active_users} />
          </dd>
        </dl>
      </div>
      <SiteBreakdownDisclosure sites={item.site_breakdown} />
      <footer className='border-border grid gap-1 border-t pt-3'>
        <span className='text-sm'>
          {t('statistics.rowCompleteness', {
            rate: Math.round(item.completeness_rate * 1000) / 10,
          })}
          {' · '}
          {item.is_final
            ? t('statistics.final.final')
            : t('statistics.final.provisional')}
        </span>
        <DataFreshness labelKey='statistics.asOf' timestamp={item.as_of} />
      </footer>
    </article>
  )
}

function BreakdownTable({
  data,
  onSearchChange,
  scope,
  search,
}: {
  data: StatisticsResponse
  onSearchChange: (changes: Partial<StatisticsSearch>) => void
  scope: StatisticsScope
  search: StatisticsSearch
}) {
  const { t } = useTranslation()
  const emptyCopy = useStatisticsEmptyCopy(data.summary.data_status)
  const columns = useMemo<ColumnDef<StatisticsBreakdownBase, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <span className='whitespace-nowrap'>
            {formatBeijingTimestamp(
              row.original.bucket_start,
              search.granularity
            )}
          </span>
        ),
        enableSorting: true,
        header: t('statistics.bucket'),
        id: 'bucket_start',
      },
      {
        cell: ({ row }) => (
          <div className='max-w-64 min-w-36'>
            <span className='font-medium break-words'>
              {dimensionLabel(row.original, scope, t)}
            </span>
            <code className='text-muted-foreground mt-1 block text-xs break-all'>
              {row.original.dimension_id}
            </code>
          </div>
        ),
        enableSorting: scope !== 'global',
        header: t('statistics.dimension'),
        id: 'name',
      },
      {
        cell: ({ row }) => <ScopeDetails item={row.original} scope={scope} />,
        header: t('statistics.scopeDetails.title'),
        id: 'scopeDetails',
      },
      {
        cell: ({ row }) => (
          <MetricValue compact value={row.original.request_count} />
        ),
        enableSorting: true,
        header: t('statistics.metric.request_count'),
        id: 'request_count',
      },
      {
        cell: ({ row }) => <MetricValue compact value={row.original.quota} />,
        enableSorting: true,
        header: t('statistics.metric.quota'),
        id: 'quota',
      },
      {
        cell: ({ row }) => (
          <AmountValue
            display={search.display}
            siteBreakdown={row.original.site_breakdown}
          />
        ),
        header: t(
          dynamicI18nKey('statistics', `statistics.display.${search.display}`)
        ),
        id: 'amount',
      },
      {
        cell: ({ row }) => (
          <MetricValue compact value={row.original.token_used} />
        ),
        enableSorting: true,
        header: t('statistics.metric.token_used'),
        id: 'token_used',
      },
      {
        cell: ({ row }) => (
          <ActiveUsersValue
            compact
            scope={scope}
            value={row.original.active_users}
          />
        ),
        enableSorting: true,
        header: t('statistics.metric.active_users'),
        id: 'active_users',
      },
      {
        cell: ({ row }) => (
          <div className='grid gap-1'>
            <DataStatusBadge status={row.original.data_status} />
            <span className='text-muted-foreground text-xs'>
              {row.original.is_final
                ? t('statistics.final.final')
                : t('statistics.final.provisional')}
            </span>
            <DataFreshness
              labelKey='statistics.asOf'
              timestamp={row.original.as_of}
            />
          </div>
        ),
        header: t('statistics.dataStatus'),
        id: 'dataStatus',
      },
    ],
    [scope, search.display, search.granularity, t]
  )
  const updateSorting = (
    updater: SortingState | ((old: SortingState) => SortingState)
  ) => {
    const current = [{ desc: search.order === 'desc', id: search.sort }]
    const next = typeof updater === 'function' ? updater(current) : updater
    const first = next[0]
    if (!first || first.id === 'amount' || first.id === 'dataStatus') return
    onSearchChange({
      order: first.desc ? 'desc' : 'asc',
      page: 1,
      sort: first.id as StatisticsSearch['sort'],
    })
  }
  const items = data.breakdown.items as StatisticsBreakdownBase[]
  return (
    <DataTable
      ariaLabel={t('statistics.breakdownTable')}
      columns={columns}
      data={items}
      emptyDescription={emptyCopy.description}
      emptyTitle={emptyCopy.title}
      onPageChange={(page) => onSearchChange({ page })}
      onPageSizeChange={(pageSize) => onSearchChange({ page: 1, pageSize })}
      onSortingChange={updateSorting}
      page={search.page}
      pageSize={search.pageSize}
      renderMobileCard={(item) => (
        <BreakdownMobileCard item={item} scope={scope} search={search} />
      )}
      sorting={[{ desc: search.order === 'desc', id: search.sort }]}
      total={data.breakdown.total}
    />
  )
}

export function StatisticsPage({
  dataSource,
  description,
  entity,
  header,
  hideScopeNavigation = false,
  onSearchChange,
  scope,
  search,
  title,
}: {
  dataSource?: StatisticsPageDataSource
  description?: string
  entity?: {
    id: IdString
    scope: 'account' | 'customer' | 'site'
  }
  header?: ReactNode
  hideScopeNavigation?: boolean
  onSearchChange: (changes: Partial<StatisticsSearch>) => void
  scope: StatisticsScope
  search: StatisticsSearch
  title?: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const params = useMemo(() => queryParams(search), [search])
  const statisticsQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () =>
      dataSource ? dataSource.query(search) : getStatistics(scope, params),
    queryKey: dataSource
      ? dataSource.queryKey(search)
      : statisticsKeys.scope(scope, params),
    staleTime: 5 * 60_000,
  })
  const response = statisticsQuery.data as StatisticsResponse | undefined
  const contractValid =
    response == null ||
    (response.scope === scope &&
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
  const [exportDraft, setExportDraft] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [recreating, setRecreating] = useState(false)
  const [exportJob, setExportJob] = useState<StatisticsExportJobItem>()
  const createExport = async (format: StatisticsExportFormat) => {
    if (!data) return
    setExporting(true)
    try {
      const request = entity
        ? buildEntityExportRequest(entity.scope, entity.id, format, search)
        : buildStatisticsExportRequest(scope, format, search)
      const job = await createStatisticsExport(request)
      toast.success(
        job.deduplicated
          ? t('statistics.export.toast.deduplicated')
          : t('statistics.export.toast.created')
      )
      queryClient.setQueryData(statisticsKeys.export(job.id), job)
      void queryClient.invalidateQueries({
        queryKey: statisticsKeys.exportLists(),
      })
      setExportJob(job)
      setExportDraft(false)
      onSearchChange({ exportId: job.id })
    } catch (error) {
      toast.error(t(dynamicI18nKey('api', getApiErrorTranslationKey(error))))
    } finally {
      setExporting(false)
    }
  }
  const recreateExport = async (job: StatisticsExportJobItem) => {
    setRecreating(true)
    try {
      const next = await createStatisticsExport({
        filters: job.filters,
        format: job.format,
        statistics_type: job.statistics_type,
      })
      toast.success(
        next.deduplicated
          ? t('statistics.export.toast.deduplicated')
          : t('statistics.export.toast.created')
      )
      queryClient.setQueryData(statisticsKeys.export(next.id), next)
      void queryClient.invalidateQueries({
        queryKey: statisticsKeys.exportLists(),
      })
      setExportJob(next)
      onSearchChange({ exportId: next.id })
    } catch (error) {
      toast.error(t(dynamicI18nKey('api', getApiErrorTranslationKey(error))))
    } finally {
      setRecreating(false)
    }
  }

  let body: ReactNode
  if (statisticsQuery.isPending && !data) {
    body = <LoadingState message={t('common.loading')} />
  } else if (
    (statisticsQuery.isError || (!contractValid && !rangeTransition)) &&
    !data
  ) {
    body = (
      <ErrorState
        description={t('statistics.loadErrorDescription')}
        onRetry={() => void statisticsQuery.refetch()}
        title={t('statistics.loadError')}
      />
    )
  } else if (!data) {
    body = null
  } else {
    const empty = data.trend.length === 0 && data.breakdown.total === 0
    let resultContent: ReactNode
    if (empty && data.summary.data_status === 'complete') {
      resultContent = (
        <section className='border-border rounded-lg border px-5 py-12 text-center'>
          <h2 className='font-medium'>{t('statistics.empty')}</h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('statistics.emptyDescription')}
          </p>
        </section>
      )
    } else if (search.view === 'chart') {
      resultContent = (
        <section
          aria-labelledby='statistics-trend-title'
          className='grid gap-3'
        >
          <h2 className='text-lg font-semibold' id='statistics-trend-title'>
            {t('statistics.trend')}
          </h2>
          <MetricTrendChart data={data.trend} search={search} />
        </section>
      )
    } else {
      resultContent = (
        <BreakdownTable
          data={data}
          onSearchChange={onSearchChange}
          scope={scope}
          search={search}
        />
      )
    }
    body = (
      <div className='grid min-w-0 gap-8'>
        {!rangeTransition && statisticsQuery.isFetching && (
          <p
            className='text-muted-foreground flex items-center gap-2 text-xs'
            role='status'
          >
            <Spinner />
            {t('table.refreshing')}
          </p>
        )}
        {rangeTransition && (
          <p
            className='border-primary/25 bg-primary/5 flex items-center gap-2 rounded-md border p-3 text-sm'
            role='status'
          >
            <Spinner />
            {t('statistics.loadingNewRange')}
          </p>
        )}
        {statisticsQuery.isError && (
          <p
            className='border-warning/40 bg-warning/10 rounded-md border p-3 text-sm'
            role='status'
          >
            {t('statistics.staleData')}
          </p>
        )}
        {resultContent}
        <CompletenessAlert completeness={data.completeness} />
      </div>
    )
  }

  return (
    <SectionPageLayout
      description={description ?? t('statistics.page.description')}
      fixedContent
      title={
        title ??
        t(dynamicI18nKey('statistics', `statistics.page.${scope}.title`))
      }
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        {header}
        {!hideScopeNavigation && (
          <ScopeNavigation scope={scope} search={search} />
        )}
        <OperationsViewPurpose
          description={t('operationsAnalytics.statistics.purpose.description')}
          icon={Chart01Icon}
          title={t('operationsAnalytics.statistics.purpose.title')}
        />
        {data && <StatisticsSummary data={data} search={search} />}
        <StatisticsToolbar
          exportDisabled={!data || rangeTransition}
          filterAction={
            entity ? undefined : (
              <StatisticsFilters
                onApply={onSearchChange}
                scope={scope}
                search={search}
              />
            )
          }
          onExportOpen={() => data && setExportDraft(true)}
          onSearchChange={onSearchChange}
          search={search}
        />
        <div className='min-h-0 flex-1 overflow-y-auto pr-1' tabIndex={0}>
          {body}
        </div>
      </div>
      {exportDraft && data && (
        <ExportDialog
          completeness={data.completeness}
          entityId={entity?.id}
          onConfirm={(format) => void createExport(format)}
          onOpenChange={setExportDraft}
          pending={exporting}
          scope={scope}
          search={search}
          summaryStatus={data.summary.data_status}
        />
      )}
      <ExportTaskSheet
        exportId={search.exportId}
        initialJob={exportJob}
        onOpenChange={(open) =>
          !open && onSearchChange({ exportId: undefined })
        }
        onRecreate={(job) => void recreateExport(job)}
        recreating={recreating}
      />
    </SectionPageLayout>
  )
}
