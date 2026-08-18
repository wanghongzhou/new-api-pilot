import {
  Add01Icon,
  Chart01Icon,
  Copy01Icon,
  Refresh01Icon,
  ServerStack01Icon,
  ViewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataViewModeToggle } from '@/components/data/data-view-mode-toggle'
import { MetricValue } from '@/components/data/metric-value'
import { QuotaAmount } from '@/components/data/quota-amount'
import { SiteStatusBadges } from '@/components/data/site-status-badges'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { PageFooterPortal } from '@/components/layout/page-footer'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { DataTablePagination } from '@/components/ui/data-table-pagination'
import { Spinner } from '@/components/ui/spinner'
import { buildStatisticsSearch } from '@/features/statistics/search'
import { useRetainedQueryData } from '@/hooks/use-retained-query-data'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { fromUnixSeconds } from '@/lib/dayjs'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { listSites, refreshSites } from '../api'
import { siteListParams } from '../list-contract'
import { siteKeys } from '../query-keys'
import {
  formatAverageRate,
  formatAverageTpm,
  formatCompletenessPercent,
  formatInstanceAvailability,
  formatPerformanceLatency,
  formatPerformanceSuccessRate,
  formatPerformanceThroughput,
  formatPercentValue,
  isSitePerformanceReady,
  sitePerformanceDashboardSummary,
} from '../site-card-metrics'
import type { SiteListItem, SiteSearch } from '../types'
import { SiteActions, type SiteAction } from './site-actions'
import { SiteCard } from './site-card'
import { SiteDialogs, type SiteDialogState } from './site-dialogs'
import { SiteFilters } from './site-filters'
import { SiteOnboardingDrawer } from './site-onboarding-drawer'

interface SitesPageProps {
  onOpenSite: (siteId: string, runId?: string) => void
  onSearchChange: (changes: Partial<SiteSearch>) => void
  search: SiteSearch
}

function ListMetric({
  children,
  label,
}: {
  children: React.ReactNode
  label: string
}) {
  return (
    <div className='min-w-0'>
      <p className='text-muted-foreground truncate text-[11px]'>{label}</p>
      <div className='text-foreground mt-1 min-w-0 font-semibold tabular-nums'>
        {children}
      </div>
    </div>
  )
}

function CompletenessBar({ label, value }: { label: string; value: number }) {
  const percent = Math.max(0, Math.min(100, value * 100))
  return (
    <div className='grid min-w-28 gap-1.5'>
      <div className='flex items-center justify-between gap-3'>
        <span className='text-muted-foreground text-xs'>{label}</span>
        <span className='text-xs font-semibold tabular-nums'>
          {formatCompletenessPercent(value)}
        </span>
      </div>
      <div
        aria-label={label}
        aria-valuemax={100}
        aria-valuemin={0}
        aria-valuenow={percent}
        className='bg-muted h-1.5 overflow-hidden rounded-full'
        role='progressbar'
      >
        <div
          className='from-primary to-success h-full rounded-full bg-gradient-to-r'
          style={{ width: `${percent}%` }}
        />
      </div>
    </div>
  )
}

function SiteIdentityCell({ site }: { site: SiteListItem }) {
  const { t } = useTranslation()
  const copyBaseUrl = async () => {
    try {
      if (!navigator.clipboard?.writeText) {
        throw new Error('clipboard unavailable')
      }
      await navigator.clipboard.writeText(site.base_url)
      toast.success(t('site.toast.baseUrlCopied'))
    } catch {
      toast.error(t('site.toast.copyFailed'))
    }
  }

  return (
    <div className='grid min-w-52 gap-2'>
      <div className='min-w-0'>
        <Link
          className='text-foreground font-semibold hover:underline'
          params={{ siteId: site.id }}
          to='/sites/$siteId'
        >
          {site.name}
        </Link>
        <div className='mt-1 flex min-w-0 items-center gap-1.5'>
          <span
            className='text-muted-foreground max-w-64 truncate font-mono text-xs'
            title={site.base_url}
          >
            {site.base_url}
          </span>
          <button
            aria-label={t('site.copyBaseUrl')}
            className='text-muted-foreground hover:text-foreground shrink-0'
            onClick={() => void copyBaseUrl()}
            title={t('site.copyBaseUrl')}
            type='button'
          >
            <HugeiconsIcon icon={Copy01Icon} size={14} strokeWidth={2} />
          </button>
        </div>
      </div>
      <SiteStatusBadges site={site} />
    </div>
  )
}

function SiteRowActions({
  isAdmin,
  onAction,
  site,
}: {
  isAdmin: boolean
  onAction: (action: SiteAction, site: SiteListItem) => void
  site: SiteListItem
}) {
  const { t } = useTranslation()
  const linkClass =
    'text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-ring flex size-9 items-center justify-center rounded-md outline-none focus-visible:ring-2'
  return (
    <div className='flex items-center justify-end gap-1'>
      <Link
        aria-label={t('site.actions.stats')}
        className={linkClass}
        params={{ siteId: site.id }}
        search={buildStatisticsSearch({})}
        title={t('site.actions.stats')}
        to='/sites/$siteId/stats'
      >
        <HugeiconsIcon icon={Chart01Icon} size={17} strokeWidth={2} />
      </Link>
      <Link
        aria-label={t('site.instanceStatus')}
        className={linkClass}
        params={{ siteId: site.id }}
        title={t('site.instanceStatus')}
        to='/sites/$siteId/status'
      >
        <HugeiconsIcon icon={ServerStack01Icon} size={17} strokeWidth={2} />
      </Link>
      <Link
        aria-label={t('site.viewDetails')}
        className={linkClass}
        params={{ siteId: site.id }}
        title={t('site.viewDetails')}
        to='/sites/$siteId'
      >
        <HugeiconsIcon icon={ViewIcon} size={17} strokeWidth={2} />
      </Link>
      {isAdmin && <SiteActions onAction={onAction} site={site} />}
    </div>
  )
}

function CardGridState({
  error,
  fetching,
  isAdmin,
  items,
  loading,
  onAction,
  onRetry,
}: {
  error: boolean
  fetching: boolean
  isAdmin: boolean
  items: SiteListItem[]
  loading: boolean
  onAction: (action: SiteAction, site: SiteListItem) => void
  onRetry: () => void
}) {
  const { t } = useTranslation()
  if (loading && items.length === 0) {
    return (
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3'>
        {Array.from({ length: 3 }, (_, index) => (
          <div
            aria-hidden='true'
            className='bg-muted/40 h-56 animate-pulse rounded-xl border'
            key={index}
          />
        ))}
      </div>
    )
  }
  if (error && items.length === 0) {
    return (
      <ErrorState
        className='border'
        description={t('table.loadErrorDescription')}
        onRetry={onRetry}
        title={t('table.loadError')}
      />
    )
  }
  if (items.length === 0) {
    return (
      <EmptyState
        bordered
        description={t('sites.emptyDescription')}
        title={t('sites.empty')}
      />
    )
  }
  return (
    <div className='grid min-w-0'>
      <div
        className={cn(
          'grid min-w-0 gap-4 transition-opacity duration-150 min-[1800px]:grid-cols-5 sm:grid-cols-2 lg:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4',
          fetching && 'pointer-events-none opacity-60'
        )}
      >
        {items.map((site) => (
          <SiteCard
            isAdmin={isAdmin}
            key={site.id}
            onAction={onAction}
            site={site}
          />
        ))}
      </div>
    </div>
  )
}

export function SitesPage({
  onOpenSite,
  onSearchChange,
  search,
}: SitesPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.user)
  const isAdmin = currentUser?.role === 'admin'
  const [onboardingOpen, setOnboardingOpen] = useState(false)
  const [dialogState, setDialogState] = useState<SiteDialogState | null>(null)
  const [batchRefreshing, setBatchRefreshing] = useState(false)

  const params = useMemo(() => siteListParams(search), [search])
  const sitesQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => listSites(params),
    queryKey: siteKeys.list(params),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })
  const pageData = useRetainedQueryData(
    sitesQuery.data,
    sitesQuery.isError,
    'sites-list'
  )
  const items = pageData?.items ?? []
  const total = pageData?.total ?? 0
  const listStale = sitesQuery.isError && pageData != null

  const invalidateSites = () => {
    void queryClient.invalidateQueries({ queryKey: siteKeys.all })
  }
  const saveView = (view: SiteSearch['view']) => {
    window.localStorage.setItem('sites:view-mode-v2', view)
    onSearchChange({ view })
  }
  const runBatchRefresh = async () => {
    if (items.length === 0) return
    setBatchRefreshing(true)
    try {
      await refreshSites(items.map((site) => site.id))
      toast.success(t('sites.refreshQueued'))
      invalidateSites()
    } catch (error) {
      toast.error(t(dynamicI18nKey('site', getApiErrorTranslationKey(error))))
    } finally {
      setBatchRefreshing(false)
    }
  }
  const updateSorting = (
    updater: SortingState | ((old: SortingState) => SortingState)
  ) => {
    const current: SortingState = [
      { desc: search.order === 'desc', id: search.sort },
    ]
    const next = typeof updater === 'function' ? updater(current) : updater
    const first = next[0]
    if (!first) return
    onSearchChange({
      order: first.desc ? 'desc' : 'asc',
      page: 1,
      sort: first.id as SiteSearch['sort'],
    })
  }

  const columns = useMemo<ColumnDef<SiteListItem, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        cell: ({ row }) => <SiteIdentityCell site={row.original} />,
        enableSorting: true,
        header: t('site.name'),
        id: 'name',
      },
      {
        cell: ({ row }) => {
          const resource = row.original.resource
          const unavailableValue = t('data.unavailableValue')
          return (
            <div className='grid min-w-48 gap-2.5'>
              <div className='grid grid-cols-2 gap-x-5 gap-y-2'>
                <ListMetric label={t('site.instances')}>
                  {formatInstanceAvailability(
                    resource.online_instance_count,
                    resource.instance_count,
                    unavailableValue
                  )}
                </ListMetric>
                <ListMetric label={t('metric.cpu')}>
                  {formatPercentValue(
                    resource.cpu_max_percent,
                    unavailableValue
                  )}
                </ListMetric>
                <ListMetric label={t('metric.memory')}>
                  {formatPercentValue(
                    resource.memory_max_percent,
                    unavailableValue
                  )}
                </ListMetric>
                <ListMetric label={t('metric.disk')}>
                  {formatPercentValue(
                    resource.disk_max_used_percent,
                    unavailableValue
                  )}
                </ListMetric>
              </div>
              <p className='text-muted-foreground text-xs whitespace-nowrap'>
                {resource.updated_at == null
                  ? t('data.noUpdateTime')
                  : t('site.resourceUpdatedAt', {
                      time: fromUnixSeconds(resource.updated_at).format(
                        'YYYY-MM-DD HH:mm:ss'
                      ),
                    })}
              </p>
            </div>
          )
        },
        header: t('site.resourceStatus'),
        id: 'resources',
      },
      {
        cell: ({ row }) => {
          const today = row.original.today
          return (
            <div className='grid min-w-64 gap-3'>
              <div className='grid grid-cols-2 gap-x-5'>
                <ListMetric label={t('site.dashboard.last24HoursQuota')}>
                  <QuotaAmount
                    emphasizeAmount
                    inline
                    nullLabel='0'
                    quota={today.quota}
                    rate={row.original.rate}
                    showQuota={false}
                  />
                </ListMetric>
                <ListMetric label={t('site.dashboard.last24HoursTokens')}>
                  <MetricValue compact nullLabel='0' value={today.token_used} />
                </ListMetric>
              </div>
              <div className='grid grid-cols-3 gap-x-5'>
                <ListMetric label={t('site.dashboard.last24HoursCount')}>
                  <MetricValue
                    compact
                    nullLabel='0'
                    value={today.request_count}
                  />
                </ListMetric>
                <ListMetric label={t('site.averageRpm')}>
                  <span title={today.avg_rpm ?? undefined}>
                    {formatAverageRate(today.avg_rpm)}
                  </span>
                </ListMetric>
                <ListMetric label={t('site.averageTpm')}>
                  <span title={today.avg_tpm ?? undefined}>
                    {formatAverageTpm(today.avg_tpm)}
                  </span>
                </ListMetric>
              </div>
            </div>
          )
        },
        header: t('site.last24HoursUsage'),
        id: 'usage_24h',
      },
      {
        cell: ({ row }) => {
          const performance = row.original.performance
          const performanceModels = performance.models ?? []
          const performanceSummary =
            sitePerformanceDashboardSummary(performanceModels)
          const unavailableValue = t('data.unavailableValue')
          if (
            !isSitePerformanceReady(performance.data_status) ||
            performanceModels.length === 0
          ) {
            return (
              <div className='grid min-w-52 gap-1'>
                <span className='font-semibold'>{unavailableValue}</span>
                <span className='text-muted-foreground text-xs'>
                  {t('site.performance.unavailable')}
                </span>
              </div>
            )
          }
          return (
            <div className='grid min-w-64 gap-2.5'>
              <div className='grid grid-cols-3 gap-x-4'>
                <ListMetric label={t('site.performance.successRate')}>
                  {formatPerformanceSuccessRate(
                    performanceSummary.successRate,
                    unavailableValue
                  )}
                </ListMetric>
                <ListMetric label={t('site.performance.avgLatency')}>
                  {formatPerformanceLatency(
                    performanceSummary.avgLatencyMs,
                    unavailableValue
                  )}
                </ListMetric>
                <ListMetric label={t('site.performance.avgTps')}>
                  {formatPerformanceThroughput(
                    performanceSummary.throughput,
                    unavailableValue
                  )}
                </ListMetric>
              </div>
              {performance.sampled_at != null && (
                <p className='text-muted-foreground text-xs whitespace-nowrap'>
                  {t('site.performance.sampledAt', {
                    time: fromUnixSeconds(performance.sampled_at).format(
                      'YYYY-MM-DD HH:mm:ss'
                    ),
                  })}
                </p>
              )}
            </div>
          )
        },
        header: t('site.performance.title'),
        id: 'performance',
      },
      {
        cell: ({ row }) => (
          <CompletenessBar
            label={t('site.completeness')}
            value={row.original.completeness_rate}
          />
        ),
        header: t('site.completeness'),
        id: 'completeness',
      },
      {
        cell: ({ row }) => (
          <SiteRowActions
            isAdmin={isAdmin}
            onAction={setDialogAction}
            site={row.original}
          />
        ),
        header: t('common.actions'),
        id: 'actions',
      },
    ],
    [isAdmin, t]
  )

  function setDialogAction(action: SiteAction, site: SiteListItem) {
    setDialogState({ action, site })
  }

  return (
    <SectionPageLayout
      actions={
        isAdmin ? (
          <>
            <Button
              disabled={batchRefreshing || items.length === 0}
              onClick={() => void runBatchRefresh()}
              variant='outline'
            >
              {batchRefreshing ? (
                <Spinner />
              ) : (
                <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
              )}
              {t('sites.refresh')}
            </Button>
            <Button onClick={() => setOnboardingOpen(true)}>
              <HugeiconsIcon icon={Add01Icon} strokeWidth={2} />
              {t('sites.create')}
            </Button>
          </>
        ) : undefined
      }
      description={t('sites.description')}
      fixedContent
      title={t('sites.title')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-5'>
        {listStale && (
          <section
            className='border-warning/40 bg-warning/10 flex flex-wrap items-center justify-between gap-3 rounded-md border p-3'
            role='status'
          >
            <div>
              <p className='font-medium'>{t('sites.refreshError')}</p>
              <p className='text-muted-foreground mt-1 text-sm'>
                {t('sites.staleData')}
              </p>
            </div>
            <Button
              disabled={sitesQuery.isFetching}
              onClick={() => void sitesQuery.refetch()}
              size='sm'
              variant='outline'
            >
              {sitesQuery.isFetching && <Spinner />}
              {t('common.retry')}
            </Button>
          </section>
        )}
        <SiteFilters
          actions={
            <DataViewModeToggle
              ariaLabel={t('sites.viewMode')}
              cardLabel={t('sites.cardView')}
              onChange={saveView}
              tableLabel={t('sites.tableView')}
              value={search.view}
            />
          }
          onApply={(filters) => onSearchChange({ ...filters, page: 1 })}
          value={search}
        />

        {search.view === 'card' ? (
          <div className='min-h-0 flex-1 overflow-y-auto' tabIndex={0}>
            <CardGridState
              error={sitesQuery.isError && !listStale}
              fetching={sitesQuery.isFetching}
              isAdmin={isAdmin}
              items={items}
              loading={sitesQuery.isPending}
              onAction={setDialogAction}
              onRetry={() => void sitesQuery.refetch()}
            />
          </div>
        ) : (
          <div className='flex min-h-0 flex-1 flex-col'>
            <DataTable
              ariaLabel={t('sites.tableLabel')}
              columns={columns}
              data={items}
              emptyDescription={t('sites.emptyDescription')}
              emptyTitle={t('sites.empty')}
              error={sitesQuery.isError && !listStale}
              fetching={sitesQuery.isFetching}
              fillAvailableHeight
              loading={sitesQuery.isPending}
              onRetry={() => void sitesQuery.refetch()}
              onSortingChange={updateSorting}
              preserveHeaderWhenEmpty
              sorting={[{ desc: search.order === 'desc', id: search.sort }]}
            />
          </div>
        )}
      </div>

      <PageFooterPortal>
        <DataTablePagination
          onPageChange={(page) => onSearchChange({ page })}
          onPageSizeChange={(pageSize) => onSearchChange({ page: 1, pageSize })}
          page={search.page}
          pageSize={search.pageSize}
          total={total}
        />
      </PageFooterPortal>

      <SiteOnboardingDrawer
        onComplete={(site, runId) => {
          invalidateSites()
          onOpenSite(site.id, runId)
        }}
        onOpenChange={setOnboardingOpen}
        open={onboardingOpen}
      />
      <SiteDialogs
        onClose={() => setDialogState(null)}
        onSaved={() => invalidateSites()}
        state={dialogState}
      />
    </SectionPageLayout>
  )
}
