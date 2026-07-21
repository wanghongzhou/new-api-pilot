import { Add01Icon, Refresh01Icon } from '@hugeicons/core-free-icons'
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
import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { QuotaAmount } from '@/components/data/quota-amount'
import { RunFeedbackSheet } from '@/components/data/run-feedback-sheet'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { listCustomers } from '@/features/customers/api'
import { customerKeys } from '@/features/customers/query-keys'
import { listSites } from '@/features/sites/api'
import { siteKeys } from '@/features/sites/query-keys'
import type { CollectionRunItem } from '@/features/sites/types'
import { useAuthStore } from '@/stores/auth-store'

import { listAccounts, refreshAccount } from '../api'
import { accountKeys } from '../query-keys'
import type {
  AccountDetail,
  AccountListItem,
  AccountListParams,
  AccountSearch,
} from '../types'
import { AccountDialogs, type AccountDialogState } from './account-dialogs'
import { AccountFilters } from './account-filters'
import { AccountOnboardingDrawer } from './account-onboarding-drawer'
import {
  AccountActions,
  AccountCard,
  ManagedStatusBadge,
  RemoteStateBadge,
  RemoteStatusBadge,
  type AccountAction,
} from './account-ui'

export function AccountsPage({
  onOpenAccount,
  onSearchChange,
  search,
}: {
  onOpenAccount: (accountId: string) => void
  onSearchChange: (changes: Partial<AccountSearch>) => void
  search: AccountSearch
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')
  const [draftFilter, setDraftFilter] = useState(search.filter)
  const [composing, setComposing] = useState(false)
  const [onboardingOpen, setOnboardingOpen] = useState(false)
  const [dialogState, setDialogState] = useState<AccountDialogState>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [recovery, setRecovery] = useState<{
    account: AccountListItem
    run: CollectionRunItem
  } | null>(null)

  useEffect(() => setDraftFilter(search.filter), [search.filter])
  useEffect(() => {
    if (composing || draftFilter === search.filter) return
    const timer = window.setTimeout(
      () => onSearchChange({ filter: draftFilter, page: 1 }),
      500
    )
    return () => window.clearTimeout(timer)
  }, [composing, draftFilter, onSearchChange, search.filter])

  const params = useMemo<AccountListParams>(
    () => ({
      customer_id: search.customerId,
      keyword: search.filter || undefined,
      managed_status:
        search.managedStatus.length > 0 ? search.managedStatus : undefined,
      p: search.page,
      page_size: search.pageSize,
      remote_state:
        search.remoteState.length > 0 ? search.remoteState : undefined,
      remote_status:
        search.remoteStatus.length > 0 ? search.remoteStatus : undefined,
      site_id: search.siteId,
      sort_by: search.sort,
      sort_order: search.order,
    }),
    [search]
  )
  const accountsQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => listAccounts(params),
    queryKey: accountKeys.list(params),
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
  const siteParams = useMemo(
    () => ({
      p: 1,
      page_size: 100,
      sort_by: 'name',
      sort_order: 'asc' as const,
    }),
    []
  )
  const sitesQuery = useQuery({
    queryFn: () => listSites(siteParams),
    queryKey: siteKeys.list(siteParams),
    staleTime: 5 * 60_000,
  })
  const customerParams = useMemo(
    () => ({
      p: 1,
      page_size: 100,
      sort_by: 'name',
      sort_order: 'asc' as const,
    }),
    []
  )
  const customersQuery = useQuery({
    queryFn: () => listCustomers(customerParams),
    queryKey: customerKeys.list(customerParams),
    staleTime: 5 * 60_000,
  })
  const accounts = accountsQuery.data?.items ?? []

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: accountKeys.all })
    void queryClient.invalidateQueries({ queryKey: customerKeys.all })
  }
  const runRefresh = async () => {
    if (accounts.length === 0) return
    setRefreshing(true)
    try {
      const results = await Promise.allSettled(
        accounts.map((account) => refreshAccount(account.id))
      )
      const failed = results.filter(
        (result) => result.status === 'rejected'
      ).length
      if (failed > 0) toast.error(t('accounts.refreshPartial', { failed }))
      else toast.success(t('accounts.refreshSuccess'))
      invalidate()
    } finally {
      setRefreshing(false)
    }
  }
  const onAction = (action: AccountAction, account: AccountListItem) =>
    setDialogState({ action, account })
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
      sort: first.id as AccountSearch['sort'],
    })
  }
  const columns = useMemo<ColumnDef<AccountListItem, unknown>[]>(
    () => [
      {
        accessorKey: 'username',
        cell: ({ row }) => (
          <div className='min-w-40'>
            <Link
              className='font-medium hover:underline'
              params={{ accountId: row.original.id }}
              to='/accounts/$accountId'
            >
              {row.original.username}
            </Link>
            <p className='text-muted-foreground text-xs'>
              {t('account.remoteUserIdValue', {
                id: row.original.remote_user_id,
              })}
            </p>
          </div>
        ),
        enableSorting: true,
        header: t('account.username'),
        id: 'username',
      },
      { accessorKey: 'site_name', header: t('account.site') },
      { accessorKey: 'customer_name', header: t('account.customer') },
      { accessorKey: 'remote_group', header: t('account.remoteGroup') },
      {
        cell: ({ row }) => (
          <RemoteStatusBadge status={row.original.remote_status} />
        ),
        header: t('account.remoteStatusLabel'),
        id: 'remoteStatus',
      },
      {
        cell: ({ row }) => (
          <RemoteStateBadge state={row.original.remote_state} />
        ),
        header: t('account.remoteStateLabel'),
        id: 'remoteState',
      },
      {
        cell: ({ row }) => (
          <ManagedStatusBadge status={row.original.managed_status} />
        ),
        header: t('account.managedStatusLabel'),
        id: 'managedStatus',
      },
      {
        cell: ({ row }) => (
          <div className='whitespace-nowrap'>
            <MetricValue compact value={row.original.quota} /> /{' '}
            <MetricValue compact value={row.original.used_quota} />
          </div>
        ),
        enableSorting: true,
        header: t('account.quotaUsed'),
        id: 'quota',
      },
      {
        cell: ({ row }) => (
          <MetricValue compact value={row.original.today.request_count} />
        ),
        header: t('account.todayRequests'),
        id: 'todayRequests',
      },
      {
        cell: ({ row }) => (
          <QuotaAmount
            quota={row.original.today.quota}
            rate={row.original.rate}
          />
        ),
        enableSorting: true,
        header: t('account.todayQuota'),
        id: 'today_quota',
      },
      {
        cell: ({ row }) => (
          <div className='grid gap-1'>
            <DataStatusBadge status={row.original.today.data_status} />
            <DataFreshness
              labelKey='account.asOf'
              timestamp={row.original.today.as_of}
            />
          </div>
        ),
        header: t('account.completeness'),
        id: 'dataStatus',
      },
      ...(isAdmin
        ? ([
            {
              cell: ({ row }) => (
                <AccountActions account={row.original} onAction={onAction} />
              ),
              header: t('common.actions'),
              id: 'actions',
            },
          ] satisfies ColumnDef<AccountListItem, unknown>[])
        : []),
    ],
    [isAdmin, t]
  )

  return (
    <SectionPageLayout
      actions={
        isAdmin ? (
          <>
            <Button
              disabled={refreshing || accounts.length === 0}
              onClick={() => void runRefresh()}
              variant='outline'
            >
              {refreshing ? (
                <Spinner />
              ) : (
                <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
              )}
              {t('accounts.refresh')}
            </Button>
            <Button onClick={() => setOnboardingOpen(true)}>
              <HugeiconsIcon icon={Add01Icon} strokeWidth={2} />
              {t('accounts.add')}
            </Button>
          </>
        ) : undefined
      }
      description={t('accounts.description')}
      title={t('accounts.title')}
    >
      <div className='grid min-w-0 gap-4'>
        <div className='flex flex-wrap items-center gap-2'>
          <Input
            aria-label={t('accounts.search')}
            className='min-w-0 flex-1 sm:max-w-sm'
            onChange={(event) => setDraftFilter(event.target.value)}
            onCompositionEnd={(event: CompositionEvent<HTMLInputElement>) => {
              setComposing(false)
              setDraftFilter(event.currentTarget.value)
            }}
            onCompositionStart={() => setComposing(true)}
            placeholder={t('accounts.searchPlaceholder')}
            value={draftFilter}
          />
        </div>
        <div>
          <AccountFilters
            customers={customersQuery.data?.items ?? []}
            onApply={(filters) => onSearchChange({ ...filters, page: 1 })}
            sites={sitesQuery.data?.items ?? []}
            value={{
              customerId: search.customerId,
              managedStatus: search.managedStatus,
              remoteState: search.remoteState,
              remoteStatus: search.remoteStatus,
              siteId: search.siteId,
            }}
          />
        </div>
        <DataTable
          ariaLabel={t('accounts.table')}
          columns={columns}
          data={accounts}
          emptyAction={
            isAdmin ? (
              <Button onClick={() => setOnboardingOpen(true)}>
                {t('accounts.add')}
              </Button>
            ) : undefined
          }
          emptyDescription={t('accounts.emptyDescription')}
          emptyTitle={t('accounts.empty')}
          error={accountsQuery.isError}
          fetching={accountsQuery.isFetching}
          loading={accountsQuery.isPending}
          onPageChange={(page) => onSearchChange({ page })}
          onPageSizeChange={(pageSize) => onSearchChange({ page: 1, pageSize })}
          onRetry={() => void accountsQuery.refetch()}
          onSortingChange={updateSorting}
          page={search.page}
          pageSize={search.pageSize}
          renderMobileCard={(account) => (
            <AccountCard
              account={account}
              isAdmin={Boolean(isAdmin)}
              onAction={onAction}
            />
          )}
          sorting={[{ desc: search.order === 'desc', id: search.sort }]}
          total={accountsQuery.data?.total ?? 0}
        />
      </div>
      <AccountOnboardingDrawer
        initialCustomerId={search.customerId}
        onComplete={(account: AccountDetail) => {
          queryClient.setQueryData(accountKeys.detail(account.id), account)
          invalidate()
          onOpenAccount(account.id)
        }}
        onOpenChange={setOnboardingOpen}
        open={onboardingOpen}
      />
      <AccountDialogs
        onClose={() => setDialogState(null)}
        onRecovery={(run, account) => setRecovery({ account, run })}
        onSaved={invalidate}
        state={dialogState}
      />
      <RunFeedbackSheet
        expectedTargetId={recovery?.account.id ?? ''}
        expectedTargetType='account'
        onOpenChange={(open) => !open && setRecovery(null)}
        open={recovery != null}
        run={recovery?.run ?? null}
      />
    </SectionPageLayout>
  )
}
