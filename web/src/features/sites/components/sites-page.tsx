import { Add01Icon, Refresh01Icon } from '@hugeicons/core-free-icons'
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

import { DataFreshness } from '@/components/data/data-freshness'
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
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { listSites, refreshSites } from '../api'
import { siteListParams } from '../list-contract'
import { siteKeys } from '../query-keys'
import { formatLatencySeconds } from '../site-card-metrics'
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

function resourceSummary(site: SiteListItem): string {
  const values = [
    site.resource.cpu_max_percent,
    site.resource.memory_max_percent,
    site.resource.disk_max_used_percent,
  ]
  return values.map((value) => `${(value ?? 0).toFixed(1)}%`).join(' / ')
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
  const pageData = sitesQuery.data
  const items = pageData?.items ?? []
  const total = pageData?.total ?? 0

  const invalidateSites = () => {
    void queryClient.invalidateQueries({ queryKey: siteKeys.all })
  }
  const saveView = (view: SiteSearch['view']) => {
    window.localStorage.setItem('sites:view-mode', view)
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
        cell: ({ row }) => (
          <div className='min-w-44'>
            <Link
              className='font-medium hover:underline'
              params={{ siteId: row.original.id }}
              to='/sites/$siteId'
            >
              {row.original.name}
            </Link>
            <p className='text-muted-foreground max-w-60 truncate text-xs'>
              {row.original.base_url}
            </p>
          </div>
        ),
        enableSorting: true,
        header: t('site.name'),
        id: 'name',
      },
      {
        cell: ({ row }) => <SiteStatusBadges site={row.original} />,
        enableSorting: true,
        header: t('site.statuses'),
        id: 'priority',
      },
      {
        cell: ({ row }) => (
          <span>
            {row.original.resource.online_instance_count ?? 0}/
            {row.original.resource.instance_count ?? 0}
          </span>
        ),
        header: t('site.instances'),
        id: 'instances',
      },
      {
        cell: ({ row }) => (
          <span className='whitespace-nowrap' title={t('site.resourceOrder')}>
            {resourceSummary(row.original)}
          </span>
        ),
        header: t('site.resources'),
        id: 'resources',
      },
      {
        cell: ({ row }) => {
          const today = row.original.today
          return (
            <div className='grid gap-1 whitespace-nowrap'>
              <span>
                {t('site.todayRequests')}:{' '}
                <MetricValue
                  compact
                  nullLabel='0'
                  value={today.request_count}
                />
              </span>
              <span>
                {t('metric.token')}:{' '}
                <MetricValue compact nullLabel='0' value={today.token_used} />
              </span>
              <span>
                {t('site.activeUsers')}:{' '}
                <MetricValue compact nullLabel='0' value={today.active_users} />
              </span>
            </div>
          )
        },
        header: t('site.todayUsage'),
        id: 'usage_24h',
      },
      {
        cell: ({ row }) => (
          <div className='grid gap-1 whitespace-nowrap'>
            <span>
              <MetricValue
                compact
                nullLabel='0'
                value={row.original.today.avg_rpm}
              />
            </span>
            <span>
              <MetricValue
                compact
                nullLabel='0'
                value={row.original.today.avg_tpm}
              />
            </span>
          </div>
        ),
        header: `${t('site.averageRpm')} / ${t('site.averageTpm')}`,
        id: 'average_throughput',
      },
      {
        cell: ({ row }) => {
          const performance = row.original.performance
          return (
            <div className='grid gap-1 whitespace-nowrap'>
              <span>
                {(performance.success_rate * 100).toFixed(1)}%{' '}
                {t('site.performance.successRate')}
              </span>
              <span>
                {t('site.performance.latencyValue', {
                  value: formatLatencySeconds(performance.avg_latency_ms),
                })}
              </span>
              <span>
                {t('site.performance.tpsValue', {
                  value: performance.avg_tps.toFixed(1),
                })}
              </span>
            </div>
          )
        },
        header: t('site.performance.title'),
        id: 'performance',
      },
      {
        cell: ({ row }) => (
          <QuotaAmount
            inline
            quota={row.original.today.quota}
            rate={row.original.rate}
          />
        ),
        enableSorting: true,
        header: t('site.todayQuota'),
        id: 'today_quota',
      },
      {
        cell: ({ row }) => (
          <div className='grid gap-1 whitespace-nowrap'>
            <span>{(row.original.completeness_rate * 100).toFixed(1)}%</span>
            <DataFreshness
              expired={row.original.realtime.expired}
              labelKey='site.currentUpdatedAt'
              timestamp={row.original.realtime.updated_at}
            />
          </div>
        ),
        header: t('site.completeness'),
        id: 'completeness',
      },
      ...(isAdmin
        ? ([
            {
              cell: ({ row }) => (
                <SiteActions onAction={setDialogAction} site={row.original} />
              ),
              header: t('common.actions'),
              id: 'actions',
            },
          ] satisfies ColumnDef<SiteListItem, unknown>[])
        : []),
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
          <div className='min-h-0 flex-1 overflow-y-auto'>
            <CardGridState
              error={sitesQuery.isError}
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
              error={sitesQuery.isError}
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
