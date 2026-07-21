import {
  Add01Icon,
  GridViewIcon,
  Refresh01Icon,
  TableIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import { useEffect, useMemo, useState, type CompositionEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataFreshness } from '@/components/data/data-freshness'
import { MetricValue } from '@/components/data/metric-value'
import { QuotaAmount } from '@/components/data/quota-amount'
import { SiteStatusBadges } from '@/components/data/site-status-badges'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { DataTablePagination } from '@/components/ui/data-table-pagination'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { listSites, refreshSites } from '../api'
import { siteKeys } from '../query-keys'
import type { SiteListItem, SiteListParams, SiteSearch } from '../types'
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
  onCreate,
  onRetry,
}: {
  error: boolean
  fetching: boolean
  isAdmin: boolean
  items: SiteListItem[]
  loading: boolean
  onAction: (action: SiteAction, site: SiteListItem) => void
  onCreate: () => void
  onRetry: () => void
}) {
  const { t } = useTranslation()
  if (loading && items.length === 0) {
    return (
      <div
        aria-hidden='true'
        className='grid gap-4 min-[1800px]:grid-cols-5 sm:grid-cols-2 lg:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4'
      >
        {Array.from({ length: 6 }, (_, index) => (
          <div
            className='border-border bg-muted/40 h-80 animate-pulse rounded-lg border'
            key={index}
          />
        ))}
      </div>
    )
  }
  if (error && items.length === 0) {
    return (
      <section className='border-destructive/30 bg-destructive/5 rounded-lg border p-5'>
        <h2 className='font-medium'>{t('sites.loadError')}</h2>
        <Button className='mt-3' onClick={onRetry} variant='outline'>
          {t('common.retry')}
        </Button>
      </section>
    )
  }
  if (items.length === 0) {
    return (
      <section className='border-border bg-card rounded-lg border p-8 text-center'>
        <h2 className='font-medium'>{t('sites.empty')}</h2>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t('sites.emptyDescription')}
        </p>
        {isAdmin && (
          <Button className='mt-4' onClick={onCreate}>
            <HugeiconsIcon icon={Add01Icon} strokeWidth={2} />
            {t('sites.create')}
          </Button>
        )}
      </section>
    )
  }
  return (
    <div className='grid gap-3'>
      {fetching && (
        <div
          aria-live='polite'
          className='text-muted-foreground flex min-h-5 items-center gap-2 text-xs'
        >
          <Spinner />
          {t('table.refreshing')}
        </div>
      )}
      <div className='grid min-w-0 gap-4 min-[1800px]:grid-cols-5 sm:grid-cols-2 lg:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4'>
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
  const [draftFilter, setDraftFilter] = useState(search.filter)
  const [composing, setComposing] = useState(false)
  const [onboardingOpen, setOnboardingOpen] = useState(false)
  const [dialogState, setDialogState] = useState<SiteDialogState | null>(null)
  const [batchRefreshing, setBatchRefreshing] = useState(false)

  useEffect(() => setDraftFilter(search.filter), [search.filter])
  useEffect(() => {
    if (composing || draftFilter === search.filter) return
    const timer = window.setTimeout(() => {
      onSearchChange({ filter: draftFilter.trim(), page: 1 })
    }, 500)
    return () => window.clearTimeout(timer)
  }, [composing, draftFilter, onSearchChange, search.filter])

  const params = useMemo<SiteListParams>(
    () => ({
      auth_status: search.auth.length > 0 ? search.auth : undefined,
      health_status: search.health.length > 0 ? search.health : undefined,
      keyword: search.filter || undefined,
      management_status:
        search.management.length > 0 ? search.management : undefined,
      online_status: search.online.length > 0 ? search.online : undefined,
      p: search.page,
      page_size: search.pageSize,
      sort_by: search.sort,
      sort_order: search.order,
      statistics_status:
        search.statistics.length > 0 ? search.statistics : undefined,
    }),
    [search]
  )
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
                nullLabel='-'
                value={
                  row.original.today.data_status === 'complete'
                    ? row.original.today.avg_rpm
                    : null
                }
              />
            </span>
            <span>
              <MetricValue
                compact
                nullLabel='-'
                value={
                  row.original.today.data_status === 'complete'
                    ? row.original.today.avg_tpm
                    : null
                }
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
          if (performance.data_status !== 'complete') return <span>-</span>
          return (
            <div className='grid gap-1 whitespace-nowrap'>
              <span>
                {(performance.success_rate * 100).toFixed(1)}%{' '}
                {t('site.performance.successRate')}
              </span>
              <span>
                {t('site.performance.latencyValue', {
                  value: performance.avg_latency_ms.toFixed(0),
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
      title={t('sites.title')}
    >
      <div className='grid min-w-0 gap-5'>
        <div className='flex min-w-0 items-center gap-2'>
          <Input
            aria-label={t('sites.search')}
            className='min-w-0 flex-1 sm:max-w-xl'
            onChange={(event) => setDraftFilter(event.target.value)}
            onCompositionEnd={(event: CompositionEvent<HTMLInputElement>) => {
              setComposing(false)
              setDraftFilter(event.currentTarget.value)
            }}
            onCompositionStart={() => setComposing(true)}
            placeholder={t('sites.searchPlaceholder')}
            value={draftFilter}
          />
        </div>
        <SiteFilters
          onApply={(filters) => onSearchChange({ ...filters, page: 1 })}
          value={search}
        />
        <div className='flex justify-end'>
          <div
            aria-label={t('sites.viewMode')}
            className='border-border flex w-fit rounded-md border p-0.5'
            role='group'
          >
            <Button
              aria-pressed={search.view === 'card'}
              onClick={() => saveView('card')}
              size='icon'
              title={t('sites.cardView')}
              variant={search.view === 'card' ? 'secondary' : 'ghost'}
            >
              <HugeiconsIcon icon={GridViewIcon} strokeWidth={2} />
            </Button>
            <Button
              aria-pressed={search.view === 'table'}
              onClick={() => saveView('table')}
              size='icon'
              title={t('sites.tableView')}
              variant={search.view === 'table' ? 'secondary' : 'ghost'}
            >
              <HugeiconsIcon icon={TableIcon} strokeWidth={2} />
            </Button>
          </div>
        </div>

        {search.view === 'card' ? (
          <>
            <CardGridState
              error={sitesQuery.isError}
              fetching={sitesQuery.isFetching}
              isAdmin={isAdmin}
              items={items}
              loading={sitesQuery.isPending}
              onAction={setDialogAction}
              onCreate={() => setOnboardingOpen(true)}
              onRetry={() => void sitesQuery.refetch()}
            />
            <DataTablePagination
              onPageChange={(page) => onSearchChange({ page })}
              onPageSizeChange={(pageSize) =>
                onSearchChange({ page: 1, pageSize })
              }
              page={search.page}
              pageSize={search.pageSize}
              total={total}
            />
          </>
        ) : (
          <DataTable
            ariaLabel={t('sites.tableLabel')}
            columns={columns}
            data={items}
            emptyAction={
              isAdmin ? (
                <Button onClick={() => setOnboardingOpen(true)}>
                  {t('sites.create')}
                </Button>
              ) : undefined
            }
            emptyDescription={t('sites.emptyDescription')}
            emptyTitle={t('sites.empty')}
            error={sitesQuery.isError}
            fetching={sitesQuery.isFetching}
            loading={sitesQuery.isPending}
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={() => void sitesQuery.refetch()}
            onSortingChange={updateSorting}
            page={search.page}
            pageSize={search.pageSize}
            sorting={[{ desc: search.order === 'desc', id: search.sort }]}
            total={total}
          />
        )}
      </div>

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
