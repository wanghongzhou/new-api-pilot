import {
  ArrowLeft01Icon,
  Chart01Icon,
  UserGroupIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { BackfillProgress } from '@/components/data/backfill-progress'
import { CompletenessAlert } from '@/components/data/completeness-alert'
import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { RunFeedbackSheet } from '@/components/data/run-feedback-sheet'
import { ErrorState } from '@/components/error-state'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { LoadingState } from '@/components/loading-state'
import { DataTable } from '@/components/ui/data-table'
import {
  AccountCard,
  ManagedStatusBadge,
  RemoteStateBadge,
  RemoteStatusBadge,
} from '@/features/accounts/components/account-ui'
import { accountKeys } from '@/features/accounts/query-keys'
import type { AccountListItem } from '@/features/accounts/types'
import type { CollectionRunItem } from '@/features/sites/types'
import { buildStatisticsSearch } from '@/features/statistics/search'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { isIdString, parseIdString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import { formatDisplayValue } from '@/lib/display-value'
import { useAuthStore } from '@/stores/auth-store'

import { getCustomer, listCustomerAccounts } from '../api'
import { customerKeys } from '../query-keys'
import { CustomerDialogs, type CustomerDialogState } from './customer-dialogs'
import {
  CustomerActions,
  CustomerQuotaAmount,
  CustomerStatusBadge,
} from './customer-ui'

function Timestamp({ value }: { value: number | null }) {
  return value == null
    ? formatDisplayValue(value)
    : fromUnixSeconds(value).format('YYYY-MM-DD HH:mm:ss')
}

function MetricCell({
  children,
  label,
}: {
  children: ReactNode
  label: string
}) {
  return (
    <div className='min-w-0 px-4 py-3'>
      <dt className='text-muted-foreground text-xs'>{label}</dt>
      <dd className='mt-1 text-lg font-semibold'>{children}</dd>
    </div>
  )
}

export function CustomerDetailPage({
  accountPage,
  customerId,
  onAccountPageChange,
  onDeleted,
}: {
  accountPage: number
  customerId: string
  onAccountPageChange: (page: number) => void
  onDeleted: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')
  const [dialogState, setDialogState] = useState<CustomerDialogState>(null)
  const [recovery, setRecovery] = useState<{
    run: CollectionRunItem
    targetId: string
  } | null>(null)
  const validCustomerId = isIdString(customerId)
  const detailQuery = useQuery({
    enabled: validCustomerId,
    queryFn: () => getCustomer(parseIdString(customerId)),
    queryKey: customerKeys.detail(customerId),
    refetchInterval: (query) =>
      query.state.data?.backfill.status === 'pending' ||
      query.state.data?.backfill.status === 'running'
        ? 5_000
        : 60_000,
    staleTime: 30_000,
  })
  const accountsQuery = useQuery({
    enabled: validCustomerId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      listCustomerAccounts(parseIdString(customerId), {
        p: accountPage,
        page_size: 10,
      }),
    queryKey: customerKeys.accounts(customerId, accountPage),
    staleTime: 30_000,
  })
  const customer = detailQuery.data
  const columns = useMemo<ColumnDef<AccountListItem, unknown>[]>(
    () => [
      {
        cell: ({ row }) => (
          <Link
            className='font-medium hover:underline'
            params={{ accountId: row.original.id }}
            to='/accounts/$accountId'
          >
            {row.original.username}
          </Link>
        ),
        header: t('account.username'),
        id: 'username',
      },
      { accessorKey: 'site_name', header: t('account.site') },
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
        cell: ({ row }) => <MetricValue compact value={row.original.quota} />,
        header: t('account.currentQuota'),
        id: 'quota',
      },
      {
        cell: ({ row }) => (
          <MetricValue compact value={row.original.today.quota} />
        ),
        header: t('account.todayQuota'),
        id: 'todayQuota',
      },
    ],
    [t]
  )

  const invalidate = () => {
    const deleted = dialogState?.action === 'delete'
    void queryClient.invalidateQueries({ queryKey: customerKeys.all })
    void queryClient.invalidateQueries({ queryKey: accountKeys.all })
    if (deleted) onDeleted()
  }
  const retry = () => {
    void detailQuery.refetch()
    void accountsQuery.refetch()
  }

  let content: ReactNode
  if (!validCustomerId || (detailQuery.isError && !customer)) {
    content = (
      <ErrorState
        description={t(
          dynamicI18nKey(
            'customer',
            validCustomerId
              ? 'customer.detail.loadErrorDescription'
              : 'customer.detail.invalidId'
          )
        )}
        onRetry={validCustomerId ? retry : undefined}
        title={t('customer.detail.loadError')}
      />
    )
  } else if (detailQuery.isPending || !customer) {
    content = <LoadingState message={t('customer.detail.loading')} />
  } else {
    content = (
      <>
        {detailQuery.isRefetchError && (
          <section
            className='border-warning/40 bg-warning/10 rounded-md border p-3'
            role='status'
          >
            <p className='font-medium'>{t('customer.detail.refreshError')}</p>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('customer.detail.staleData')}
            </p>
          </section>
        )}
        <section
          aria-labelledby='customer-profile-title'
          className='grid gap-3'
        >
          <div className='flex flex-wrap items-center gap-2'>
            <h2 className='text-lg font-semibold' id='customer-profile-title'>
              {t('customer.detail.profile')}
            </h2>
            <CustomerStatusBadge status={customer.status} />
          </div>
          <dl className='grid gap-x-6 gap-y-3 border-t pt-4 sm:grid-cols-2 lg:grid-cols-3'>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('customer.contractAmount')}
              </dt>
              <dd className='mt-1 text-sm font-medium break-words'>
                {customer.contract_amount}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('customer.paymentAmount')}
              </dt>
              <dd className='mt-1 text-sm font-medium break-words'>
                {customer.payment_amount}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('customer.contact')}
              </dt>
              <dd className='mt-1 text-sm font-medium break-words'>
                {formatDisplayValue(customer.contact)}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('customer.remark')}
              </dt>
              <dd className='mt-1 text-sm font-medium break-words'>
                {formatDisplayValue(customer.remark)}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('customer.statisticsPausedAt')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                <Timestamp value={customer.statistics_paused_at} />
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('common.createdAt')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                <Timestamp value={customer.created_at} />
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('common.updatedAt')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                <Timestamp value={customer.updated_at} />
              </dd>
            </div>
          </dl>
        </section>

        <section
          aria-labelledby='customer-summary-title'
          className='grid gap-3'
        >
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <h2 className='text-lg font-semibold' id='customer-summary-title'>
              {t('customer.detail.todaySummary')}
            </h2>
            <DataFreshness
              labelKey='customer.asOf'
              timestamp={customer.today.as_of}
            />
          </div>
          <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 lg:grid-cols-4 [&>div]:border-r [&>div]:border-b'>
            <MetricCell label={t('customer.accounts')}>
              {customer.active_account_count}/{customer.account_count}
            </MetricCell>
            <MetricCell label={t('customer.sites')}>
              {customer.site_count}
            </MetricCell>
            <MetricCell label={t('customer.todayRequests')}>
              <MetricValue value={customer.today.request_count} />
            </MetricCell>
            <MetricCell label={t('customer.activeAccounts')}>
              <MetricValue value={customer.today.active_users} />
            </MetricCell>
          </dl>
          <div className='flex flex-wrap items-start justify-between gap-3 border-b pb-4'>
            <CustomerQuotaAmount customer={customer} />
            <DataStatusBadge status={customer.today.data_status} />
          </div>
        </section>

        <div className='grid gap-4 lg:grid-cols-2'>
          <CompletenessAlert completeness={customer.completeness} />
          <BackfillProgress backfill={customer.backfill} />
        </div>

        <section
          aria-labelledby='customer-accounts-title'
          className='grid gap-3'
        >
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <div>
              <h2
                className='text-lg font-semibold'
                id='customer-accounts-title'
              >
                {t('customer.detail.accounts')}
              </h2>
              <p className='text-muted-foreground mt-1 text-sm'>
                {t('customer.detail.accountsDescription')}
              </p>
            </div>
            <Link
              className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
              search={{
                customer_id: customer.id,
                managedStatus: [],
                remoteState: [],
                remoteStatus: [],
              }}
              to='/accounts'
            >
              <HugeiconsIcon icon={UserGroupIcon} strokeWidth={2} />
              {t('customer.actions.accounts')}
            </Link>
          </div>
          <DataTable
            ariaLabel={t('customer.detail.accountsTable')}
            columns={columns}
            data={accountsQuery.data?.items ?? []}
            emptyDescription={t('customer.detail.accountsEmptyDescription')}
            emptyTitle={t('customer.detail.accountsEmpty')}
            error={accountsQuery.isError}
            fetching={accountsQuery.isFetching}
            loading={accountsQuery.isPending}
            onPageChange={onAccountPageChange}
            onRetry={() => void accountsQuery.refetch()}
            page={accountPage}
            pageSize={10}
            renderMobileCard={(account) => (
              <AccountCard
                account={account}
                isAdmin={false}
                onAction={() => undefined}
              />
            )}
            total={accountsQuery.data?.total ?? 0}
          />
        </section>
      </>
    )
  }

  return (
    <SectionPageLayout
      actions={
        customer ? (
          <>
            <Link
              className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
              params={{ customerId }}
              search={buildStatisticsSearch({})}
              to='/customers/$customerId/stats'
            >
              <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
              {t('customer.actions.stats')}
            </Link>
            {isAdmin && (
              <CustomerActions
                customer={customer}
                onAction={(action, selectedCustomer) =>
                  setDialogState({ action, customer: selectedCustomer })
                }
              />
            )}
          </>
        ) : undefined
      }
      description={customer?.contact || t('customer.detail.description')}
      title={customer?.name ?? t('customer.detail.title')}
    >
      <div className='grid min-w-0 gap-8'>
        <DetailBackLink
          render={<Link search={{ status: [] }} to='/customers' />}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          {t('customer.backToList')}
        </DetailBackLink>
        {content}
      </div>
      <CustomerDialogs
        onClose={() => setDialogState(null)}
        onRecovery={(run, selectedCustomer) =>
          setRecovery({ run, targetId: selectedCustomer.id })
        }
        onSaved={invalidate}
        state={dialogState}
      />
      <RunFeedbackSheet
        expectedTargetId={recovery?.targetId ?? ''}
        expectedTargetType='customer'
        onOpenChange={(open) => !open && setRecovery(null)}
        open={recovery != null}
        run={recovery?.run ?? null}
      />
    </SectionPageLayout>
  )
}
