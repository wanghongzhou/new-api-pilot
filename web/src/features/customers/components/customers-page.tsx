import {
  Add01Icon,
  Chart01Icon,
  UserGroupIcon,
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
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { DataViewModeToggle } from '@/components/data/data-view-mode-toggle'
import { MetricValue } from '@/components/data/metric-value'
import { RunFeedbackSheet } from '@/components/data/run-feedback-sheet'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { PageFooterPortal } from '@/components/layout/page-footer'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { DataTablePagination } from '@/components/ui/data-table-pagination'
import { Spinner } from '@/components/ui/spinner'
import { accountKeys } from '@/features/accounts/query-keys'
import { formatAverageRate } from '@/features/sites/site-card-metrics'
import type { CollectionRunItem } from '@/features/sites/types'
import { statisticsKeys } from '@/features/statistics/query-keys'
import { buildStatisticsSearch } from '@/features/statistics/search'
import { useLastValidPage } from '@/hooks/use-last-valid-page'
import { useRetainedQueryData } from '@/hooks/use-retained-query-data'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { formatDecimalDisplayValue } from '@/lib/display-value'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { listCustomers } from '../api'
import { customerKeys } from '../query-keys'
import type {
  CustomerListItem,
  CustomerListParams,
  CustomerSearch,
} from '../types'
import { CustomerDialogs, type CustomerDialogState } from './customer-dialogs'
import { CustomerFilters } from './customer-filters'
import {
  CustomerActions,
  CustomerCard,
  CustomerQuotaAmount,
  CustomerStatusBadge,
  type CustomerAction,
} from './customer-ui'

function ListMetric({
  children,
  label,
}: {
  children: ReactNode
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

function CustomerCardGridState({
  error,
  fetching,
  isAdmin,
  items,
  loading,
  onAction,
  onOpenAccounts,
  onRetry,
  emptyDescription,
  emptyTitle,
}: {
  error: boolean
  fetching: boolean
  isAdmin: boolean
  items: CustomerListItem[]
  loading: boolean
  onAction: (action: CustomerAction, customer: CustomerListItem) => void
  onOpenAccounts: (customerId: string) => void
  onRetry: () => void
  emptyDescription: string
  emptyTitle: string
}) {
  const { t } = useTranslation()
  if (loading && items.length === 0) {
    return (
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3'>
        {Array.from({ length: 3 }, (_, index) => (
          <div
            aria-hidden='true'
            className='bg-muted/40 h-56 animate-pulse rounded-lg border'
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
      <EmptyState bordered description={emptyDescription} title={emptyTitle} />
    )
  }
  return (
    <div className='grid min-w-0'>
      <div
        className={cn(
          'grid min-w-0 gap-4 transition-opacity duration-150 min-[1800px]:grid-cols-5 sm:grid-cols-2 lg:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4',
          fetching && 'opacity-70'
        )}
      >
        {items.map((customer) => (
          <CustomerCard
            customer={customer}
            isAdmin={isAdmin}
            key={customer.id}
            onAction={onAction}
            onOpenAccounts={onOpenAccounts}
          />
        ))}
      </div>
    </div>
  )
}

export function CustomersPage({
  onOpenAccounts,
  onPageReplace,
  onSearchChange,
  search,
}: {
  onOpenAccounts: (customerId: string) => void
  onPageReplace: (page: number) => void
  onSearchChange: (changes: Partial<CustomerSearch>) => void
  search: CustomerSearch
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')
  const [dialogState, setDialogState] = useState<CustomerDialogState>(null)
  const [recovery, setRecovery] = useState<{
    customer: CustomerListItem
    run: CollectionRunItem
  } | null>(null)

  const params = useMemo<CustomerListParams>(
    () => ({
      keyword: search.filter || undefined,
      p: search.page,
      page_size: search.pageSize,
      sort_by: search.sort,
      sort_order: search.order,
      status: search.status.length > 0 ? search.status : undefined,
    }),
    [search]
  )
  const customersQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => listCustomers(params),
    queryKey: customerKeys.list(params),
    refetchInterval: (query) =>
      query.state.data?.items.some(
        (item) =>
          item.backfill.status === 'pending' ||
          item.backfill.status === 'running'
      )
        ? 5_000
        : 60_000,
    staleTime: 30_000,
  })
  const retainedCustomers = useRetainedQueryData(
    customersQuery.data,
    customersQuery.isError,
    'customers-list'
  )
  const customers = retainedCustomers?.items ?? []
  const total = retainedCustomers?.total ?? 0
  const listStale = customersQuery.isError && retainedCustomers != null
  const hasActiveFilters =
    search.filter.trim().length > 0 || search.status.length > 0
  const emptyTitle = t(
    dynamicI18nKey(
      'customer',
      hasActiveFilters ? 'customers.noResults' : 'customers.empty'
    )
  )
  const emptyDescription = t(
    dynamicI18nKey(
      'customer',
      hasActiveFilters
        ? 'customers.noResultsDescription'
        : 'customers.emptyDescription'
    )
  )

  useLastValidPage({
    isFetching: customersQuery.isFetching,
    isPlaceholderData: customersQuery.isPlaceholderData,
    onReplace: onPageReplace,
    page: search.page,
    pageSize: search.pageSize,
    total: retainedCustomers?.total,
  })

  const onAction = (action: CustomerAction, customer: CustomerListItem) =>
    setDialogState({ action, customer })
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: customerKeys.all })
    void queryClient.invalidateQueries({ queryKey: accountKeys.all })
    void queryClient.invalidateQueries({ queryKey: statisticsKeys.all })
  }
  const updateSorting = (
    updater: SortingState | ((old: SortingState) => SortingState)
  ) => {
    const current = [{ desc: search.order === 'desc', id: search.sort }]
    const next = typeof updater === 'function' ? updater(current) : updater
    const first = next[0]
    if (!first) return
    onSearchChange({
      order: first.desc ? 'desc' : 'asc',
      page: 1,
      sort: first.id as CustomerSearch['sort'],
    })
  }
  const columns = useMemo<ColumnDef<CustomerListItem, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        cell: ({ row }) => (
          <div className='grid min-w-48 gap-2'>
            <Link
              className='text-base leading-tight font-semibold hover:underline'
              params={{ customerId: row.original.id }}
              to='/customers/$customerId'
            >
              {row.original.name}
            </Link>
            <p className='text-muted-foreground max-w-60 truncate text-xs'>
              {row.original.contact || '-'}
            </p>
            <CustomerStatusBadge status={row.original.status} />
          </div>
        ),
        enableSorting: true,
        header: t('customer.list.identity'),
        id: 'name',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-56 grid-cols-2 gap-x-4 gap-y-3'>
            <ListMetric label={t('customer.contractAmount')}>
              <span title={row.original.contract_amount}>
                {formatDecimalDisplayValue(row.original.contract_amount)}
              </span>
            </ListMetric>
            <ListMetric label={t('customer.paymentAmount')}>
              <span title={row.original.payment_amount}>
                {formatDecimalDisplayValue(row.original.payment_amount)}
              </span>
            </ListMetric>
            <ListMetric label={t('customer.activeTotalAccounts')}>
              {row.original.active_account_count}/{row.original.account_count}
            </ListMetric>
            <ListMetric label={t('customer.sites')}>
              {row.original.site_count}
            </ListMetric>
          </div>
        ),
        enableSorting: true,
        header: t('customer.list.business'),
        id: 'account_count',
      },
      {
        cell: ({ row }) => (
          <div className='grid min-w-72 gap-3'>
            <div className='grid grid-cols-2 gap-x-4'>
              <ListMetric label={t('site.dashboard.todayQuota')}>
                <CustomerQuotaAmount customer={row.original} />
              </ListMetric>
              <ListMetric label={t('site.dashboard.todayTokens')}>
                <MetricValue
                  compact
                  nullLabel='0'
                  value={row.original.today.token_used}
                />
              </ListMetric>
            </div>
            <div className='grid grid-cols-3 gap-x-4'>
              <ListMetric label={t('site.dashboard.todayCount')}>
                <MetricValue
                  compact
                  nullLabel='0'
                  value={row.original.today.request_count}
                />
              </ListMetric>
              <ListMetric label={t('site.averageRpm')}>
                {formatAverageRate(row.original.today.avg_rpm)}
              </ListMetric>
              <ListMetric label={t('site.averageTpm')}>
                {formatAverageRate(row.original.today.avg_tpm)}
              </ListMetric>
            </div>
            <p className='text-muted-foreground text-xs'>
              {t('customer.activeAccounts')}:{' '}
              <MetricValue
                compact
                nullLabel='0'
                value={row.original.today.active_users}
              />
            </p>
          </div>
        ),
        enableSorting: true,
        header: t('customer.list.todayUsage'),
        id: 'today_quota',
      },
      {
        cell: ({ row }) => (
          <div className='grid gap-1'>
            <DataStatusBadge status={row.original.today.data_status} />
            <DataFreshness
              labelKey='customer.asOf'
              timestamp={row.original.today.as_of}
            />
          </div>
        ),
        header: t('customer.list.dataStatus'),
        id: 'dataStatus',
      },
      {
        cell: ({ row }) => (
          <div className='flex items-center gap-1'>
            <Link
              aria-label={t('customer.actions.stats')}
              className='hover:bg-muted inline-flex size-8 items-center justify-center rounded-md'
              params={{ customerId: row.original.id }}
              search={buildStatisticsSearch({})}
              title={t('customer.actions.stats')}
              to='/customers/$customerId/stats'
            >
              <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
            </Link>
            <Button
              aria-label={t('customer.actions.accounts')}
              onClick={() => onOpenAccounts(row.original.id)}
              size='icon'
              title={t('customer.actions.accounts')}
              variant='ghost'
            >
              <HugeiconsIcon icon={UserGroupIcon} strokeWidth={2} />
            </Button>
            <Link
              aria-label={t('customer.actions.detail')}
              className='hover:bg-muted inline-flex size-8 items-center justify-center rounded-md'
              params={{ customerId: row.original.id }}
              title={t('customer.actions.detail')}
              to='/customers/$customerId'
            >
              <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
            </Link>
            {isAdmin && (
              <CustomerActions customer={row.original} onAction={onAction} />
            )}
          </div>
        ),
        header: t('common.actions'),
        id: 'actions',
      },
    ],
    [isAdmin, onOpenAccounts, t]
  )

  return (
    <SectionPageLayout
      fixedContent
      actions={
        isAdmin ? (
          <Button onClick={() => setDialogState({ action: 'create' })}>
            <HugeiconsIcon icon={Add01Icon} strokeWidth={2} />
            {t('customers.create')}
          </Button>
        ) : undefined
      }
      description={t('customers.description')}
      title={t('customers.title')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-5'>
        {listStale && (
          <section
            className='border-warning/40 bg-warning/10 flex flex-wrap items-center justify-between gap-3 rounded-md border p-3'
            role='status'
          >
            <div>
              <p className='font-medium'>{t('customers.refreshError')}</p>
              <p className='text-muted-foreground mt-1 text-sm'>
                {t('customers.staleData')}
              </p>
            </div>
            <Button
              disabled={customersQuery.isFetching}
              onClick={() => void customersQuery.refetch()}
              size='sm'
              variant='outline'
            >
              {customersQuery.isFetching && <Spinner />}
              {t('common.retry')}
            </Button>
          </section>
        )}
        <CustomerFilters
          actions={
            <DataViewModeToggle
              ariaLabel={t('customers.viewMode')}
              cardLabel={t('customers.cardView')}
              onChange={(view) => onSearchChange({ view })}
              tableLabel={t('customers.tableView')}
              value={search.view}
            />
          }
          onApply={(filters) => onSearchChange({ ...filters, page: 1 })}
          value={{ filter: search.filter, status: search.status }}
        />
        {search.view === 'card' ? (
          <div className='min-h-0 flex-1 overflow-y-auto' tabIndex={0}>
            <CustomerCardGridState
              error={customersQuery.isError && !listStale}
              emptyDescription={emptyDescription}
              emptyTitle={emptyTitle}
              fetching={customersQuery.isFetching}
              isAdmin={Boolean(isAdmin)}
              items={customers}
              loading={customersQuery.isPending}
              onAction={onAction}
              onOpenAccounts={onOpenAccounts}
              onRetry={() => void customersQuery.refetch()}
            />
          </div>
        ) : (
          <div className='flex min-h-0 flex-1 flex-col'>
            <DataTable
              ariaLabel={t('customers.table')}
              columns={columns}
              data={customers}
              emptyDescription={emptyDescription}
              emptyTitle={emptyTitle}
              error={customersQuery.isError && !listStale}
              fetching={customersQuery.isFetching}
              fillAvailableHeight
              loading={customersQuery.isPending}
              onRetry={() => void customersQuery.refetch()}
              renderMobileCard={(customer) => (
                <CustomerCard
                  customer={customer}
                  isAdmin={Boolean(isAdmin)}
                  onAction={onAction}
                  onOpenAccounts={onOpenAccounts}
                />
              )}
              rowHeaderColumnId='name'
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
      <CustomerDialogs
        onClose={() => setDialogState(null)}
        onRecovery={(run, customer) => setRecovery({ customer, run })}
        onSaved={invalidate}
        state={dialogState}
      />
      <RunFeedbackSheet
        expectedTargetId={recovery?.customer.id ?? ''}
        expectedTargetType='customer'
        onOpenChange={(open) => !open && setRecovery(null)}
        open={recovery != null}
        run={recovery?.run ?? null}
      />
    </SectionPageLayout>
  )
}
