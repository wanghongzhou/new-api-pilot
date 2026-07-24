import { Add01Icon, ViewIcon } from '@hugeicons/core-free-icons'
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

import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { DataViewModeToggle } from '@/components/data/data-view-mode-toggle'
import { MetricValue } from '@/components/data/metric-value'
import { RunFeedbackSheet } from '@/components/data/run-feedback-sheet'
import { PageFooterPortal } from '@/components/layout/page-footer'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { DataTablePagination } from '@/components/ui/data-table-pagination'
import { accountKeys } from '@/features/accounts/query-keys'
import type { CollectionRunItem } from '@/features/sites/types'
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

export function CustomersPage({
  onOpenAccounts,
  onSearchChange,
  search,
}: {
  onOpenAccounts: (customerId: string) => void
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
  const customers = customersQuery.data?.items ?? []
  const total = customersQuery.data?.total ?? 0

  const onAction = (action: CustomerAction, customer: CustomerListItem) =>
    setDialogState({ action, customer })
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: customerKeys.all })
    void queryClient.invalidateQueries({ queryKey: accountKeys.all })
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
          <div className='min-w-44'>
            <Link
              className='font-medium hover:underline'
              params={{ customerId: row.original.id }}
              to='/customers/$customerId'
            >
              {row.original.name}
            </Link>
            <p className='text-muted-foreground max-w-60 truncate text-xs'>
              {row.original.contact || '-'}
            </p>
          </div>
        ),
        enableSorting: true,
        header: t('customer.name'),
        id: 'name',
      },
      {
        cell: ({ row }) => <CustomerStatusBadge status={row.original.status} />,
        header: t('customer.statusLabel'),
        id: 'status',
      },
      {
        accessorKey: 'contract_amount',
        header: t('customer.contractAmount'),
      },
      {
        accessorKey: 'payment_amount',
        header: t('customer.paymentAmount'),
      },
      {
        cell: ({ row }) =>
          `${row.original.active_account_count}/${row.original.account_count}`,
        enableSorting: true,
        header: t('customer.activeTotalAccounts'),
        id: 'account_count',
      },
      {
        accessorKey: 'site_count',
        header: t('customer.sites'),
      },
      {
        cell: ({ row }) => (
          <MetricValue compact value={row.original.today.request_count} />
        ),
        header: t('customer.todayRequests'),
        id: 'requests',
      },
      {
        cell: ({ row }) => <CustomerQuotaAmount customer={row.original} />,
        enableSorting: true,
        header: t('customer.todayQuota'),
        id: 'today_quota',
      },
      {
        cell: ({ row }) => (
          <MetricValue compact value={row.original.today.active_users} />
        ),
        header: t('customer.activeAccounts'),
        id: 'activeUsers',
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
        header: t('customer.completeness'),
        id: 'dataStatus',
      },
      {
        cell: ({ row }) => (
          <div className='flex items-center gap-1'>
            <Button
              aria-label={t('customer.actions.accounts')}
              onClick={() => onOpenAccounts(row.original.id)}
              size='icon'
              title={t('customer.actions.accounts')}
              variant='ghost'
            >
              <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
            </Button>
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
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-4'>
        <CustomerFilters
          onApply={(filters) => onSearchChange({ ...filters, page: 1 })}
          value={{ filter: search.filter, status: search.status }}
        />
        <div className='flex justify-end'>
          <DataViewModeToggle
            ariaLabel={t('customers.viewMode')}
            cardLabel={t('customers.cardView')}
            onChange={(view) => onSearchChange({ view })}
            tableLabel={t('customers.tableView')}
            value={search.view}
          />
        </div>
        {search.view === 'card' && customers.length > 0 ? (
          <>
            <div className='grid min-h-0 flex-1 gap-3 overflow-y-auto sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4'>
              {customers.map((customer) => (
                <CustomerCard
                  customer={customer}
                  isAdmin={Boolean(isAdmin)}
                  key={customer.id}
                  onAction={onAction}
                  onOpenAccounts={onOpenAccounts}
                />
              ))}
            </div>
            <PageFooterPortal>
              <DataTablePagination
                onPageChange={(page) => onSearchChange({ page })}
                onPageSizeChange={(pageSize) =>
                  onSearchChange({ page: 1, pageSize })
                }
                page={search.page}
                pageSize={search.pageSize}
                total={total}
              />
            </PageFooterPortal>
          </>
        ) : (
          <DataTable
            ariaLabel={t('customers.table')}
            columns={columns}
            data={customers}
            emptyDescription={t('customers.emptyDescription')}
            emptyTitle={t('customers.empty')}
            error={customersQuery.isError}
            fetching={customersQuery.isFetching}
            loading={customersQuery.isPending}
            onPageChange={(page) => onSearchChange({ page })}
            onPageSizeChange={(pageSize) =>
              onSearchChange({ page: 1, pageSize })
            }
            onRetry={() => void customersQuery.refetch()}
            onSortingChange={updateSorting}
            page={search.page}
            pageSize={search.pageSize}
            sorting={[{ desc: search.order === 'desc', id: search.sort }]}
            total={total}
          />
        )}
      </div>
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
