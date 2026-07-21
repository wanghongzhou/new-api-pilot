import {
  Alert02Icon,
  ArrowLeft01Icon,
  Chart01Icon,
  Refresh01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { BackfillProgress } from '@/components/data/backfill-progress'
import { CompletenessAlert } from '@/components/data/completeness-alert'
import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { QuotaAmount } from '@/components/data/quota-amount'
import { RunFeedbackSheet } from '@/components/data/run-feedback-sheet'
import { DetailBackLink } from '@/components/layout/detail-back-link'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { customerKeys } from '@/features/customers/query-keys'
import type { CollectionRunItem } from '@/features/sites/types'
import { buildStatisticsSearch } from '@/features/statistics/search'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { isIdString, parseIdString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import { useAuthStore } from '@/stores/auth-store'

import { getAccount } from '../api'
import { accountKeys } from '../query-keys'
import { AccountDialogs, type AccountDialogState } from './account-dialogs'
import {
  AccountActions,
  ManagedStatusBadge,
  RemoteStateBadge,
  RemoteStatusBadge,
} from './account-ui'

function Timestamp({ value }: { value: number | null }) {
  const { t } = useTranslation()
  return value == null
    ? t('common.none')
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

export function AccountDetailPage({
  accountId,
  onDeleted,
}: {
  accountId: string
  onDeleted: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')
  const [dialogState, setDialogState] = useState<AccountDialogState>(null)
  const [recovery, setRecovery] = useState<CollectionRunItem | null>(null)
  const validAccountId = isIdString(accountId)
  const detailQuery = useQuery({
    enabled: validAccountId,
    queryFn: () => getAccount(parseIdString(accountId)),
    queryKey: accountKeys.detail(accountId),
    refetchInterval: (query) =>
      query.state.data?.backfill.status === 'pending' ||
      query.state.data?.backfill.status === 'running'
        ? 5_000
        : 60_000,
    staleTime: 30_000,
  })
  const account = detailQuery.data
  const invalidate = () => {
    const deleted = dialogState?.action === 'delete'
    void queryClient.invalidateQueries({ queryKey: accountKeys.all })
    void queryClient.invalidateQueries({ queryKey: customerKeys.all })
    if (deleted) onDeleted()
  }

  let content: ReactNode
  if (!validAccountId || (detailQuery.isError && !account)) {
    content = (
      <section className='border-destructive/30 bg-destructive/5 rounded-lg border p-5'>
        <h2 className='font-medium'>{t('account.detail.loadError')}</h2>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t(
            dynamicI18nKey(
              'account',
              validAccountId
                ? 'account.detail.loadErrorDescription'
                : 'account.detail.invalidId'
            )
          )}
        </p>
        {validAccountId && (
          <Button
            className='mt-3'
            onClick={() => void detailQuery.refetch()}
            variant='outline'
          >
            <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
            {t('common.retry')}
          </Button>
        )}
      </section>
    )
  } else if (detailQuery.isPending || !account) {
    content = (
      <div className='flex min-h-64 items-center justify-center' role='status'>
        <Spinner />
        <span className='sr-only'>{t('account.detail.loading')}</span>
      </div>
    )
  } else {
    content = (
      <>
        {detailQuery.isRefetchError && (
          <section
            className='border-warning/40 bg-warning/10 rounded-md border p-3'
            role='status'
          >
            <p className='font-medium'>{t('account.detail.refreshError')}</p>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('account.detail.staleData')}
            </p>
          </section>
        )}
        {account.remote_state !== 'normal' && (
          <section
            className='border-warning/40 bg-warning/10 rounded-md border p-4'
            role='alert'
          >
            <h2 className='flex items-center gap-2 font-medium'>
              <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
              {t(
                dynamicI18nKey(
                  'account',
                  `account.detail.remoteAlert.${account.remote_state}.title`
                )
              )}
            </h2>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t(
                dynamicI18nKey(
                  'account',
                  `account.detail.remoteAlert.${account.remote_state}.description`
                ),
                {
                  count: account.remote_missing_count,
                }
              )}
            </p>
          </section>
        )}

        <section aria-labelledby='account-binding-title' className='grid gap-3'>
          <div className='flex flex-wrap items-center gap-2'>
            <h2 className='text-lg font-semibold' id='account-binding-title'>
              {t('account.detail.binding')}
            </h2>
            <Badge variant='neutral'>
              {t('account.detail.bindingImmutable')}
            </Badge>
          </div>
          <dl className='grid gap-x-6 gap-y-3 border-t pt-4 sm:grid-cols-2 lg:grid-cols-3'>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('account.site')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                <Link
                  className='hover:underline'
                  params={{ siteId: account.site_id }}
                  to='/sites/$siteId'
                >
                  {account.site_name}
                </Link>
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('account.customer')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                <Link
                  className='hover:underline'
                  params={{ customerId: account.customer_id }}
                  to='/customers/$customerId'
                >
                  {account.customer_name}
                </Link>
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('account.remoteUserId')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                {account.remote_user_id}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('account.remoteCreatedAt')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                <Timestamp value={account.remote_created_at} />
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('account.remark')}
              </dt>
              <dd className='mt-1 text-sm font-medium break-words'>
                {account.remark || t('common.none')}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('common.createdAt')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                <Timestamp value={account.created_at} />
              </dd>
            </div>
          </dl>
        </section>

        <section aria-labelledby='account-remote-title' className='grid gap-3'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <h2 className='text-lg font-semibold' id='account-remote-title'>
              {t('account.detail.currentRemote')}
            </h2>
            <DataFreshness
              labelKey='account.lastSyncedAt'
              timestamp={account.last_synced_at}
            />
          </div>
          <div className='flex flex-wrap gap-2'>
            <RemoteStatusBadge status={account.remote_status} />
            <RemoteStateBadge state={account.remote_state} />
            <ManagedStatusBadge status={account.managed_status} />
          </div>
          <dl className='grid gap-x-6 gap-y-3 border-t pt-4 sm:grid-cols-2 lg:grid-cols-4'>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('account.username')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>{account.username}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('account.displayName')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                {account.display_name || t('common.none')}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('account.remoteGroup')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                {account.remote_group || t('common.none')}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('account.lastRemoteSeenAt')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                <Timestamp value={account.last_remote_seen_at} />
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('account.statisticsPausedAt')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                <Timestamp value={account.statistics_paused_at} />
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>
                {t('common.updatedAt')}
              </dt>
              <dd className='mt-1 text-sm font-medium'>
                <Timestamp value={account.updated_at} />
              </dd>
            </div>
          </dl>
        </section>

        <section aria-labelledby='account-summary-title' className='grid gap-3'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <h2 className='text-lg font-semibold' id='account-summary-title'>
              {t('account.detail.todaySummary')}
            </h2>
            <DataFreshness
              labelKey='account.asOf'
              timestamp={account.today.as_of}
            />
          </div>
          <dl className='border-border grid overflow-hidden rounded-lg border sm:grid-cols-2 lg:grid-cols-4 [&>div]:border-r [&>div]:border-b'>
            <MetricCell label={t('account.currentQuota')}>
              <MetricValue value={account.quota} />
            </MetricCell>
            <MetricCell label={t('account.usedQuota')}>
              <MetricValue value={account.used_quota} />
            </MetricCell>
            <MetricCell label={t('account.todayRequests')}>
              <MetricValue value={account.today.request_count} />
            </MetricCell>
            <MetricCell label={t('metric.token')}>
              <MetricValue value={account.today.token_used} />
            </MetricCell>
          </dl>
          <div className='flex flex-wrap items-start justify-between gap-3 border-b pb-4'>
            <QuotaAmount quota={account.today.quota} rate={account.rate} />
            <DataStatusBadge status={account.today.data_status} />
          </div>
        </section>

        <div className='grid gap-4 lg:grid-cols-2'>
          <CompletenessAlert completeness={account.completeness} />
          <BackfillProgress backfill={account.backfill} />
        </div>
      </>
    )
  }

  return (
    <SectionPageLayout
      actions={
        account ? (
          <>
            <Link
              className='border-border hover:bg-muted inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium'
              params={{ accountId }}
              search={buildStatisticsSearch({})}
              to='/accounts/$accountId/stats'
            >
              <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
              {t('account.actions.stats')}
            </Link>
            {isAdmin && (
              <AccountActions
                account={account}
                onAction={(action, selectedAccount) =>
                  setDialogState({ account: selectedAccount, action })
                }
              />
            )}
          </>
        ) : undefined
      }
      description={
        account
          ? `${account.site_name} / ${account.customer_name}`
          : t('account.detail.description')
      }
      title={account?.username ?? t('account.detail.title')}
    >
      <div className='grid min-w-0 gap-8'>
        <DetailBackLink
          render={
            <Link
              search={{ managedStatus: [], remoteState: [], remoteStatus: [] }}
              to='/accounts'
            />
          }
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          {t('account.backToList')}
        </DetailBackLink>
        {content}
      </div>
      <AccountDialogs
        onClose={() => setDialogState(null)}
        onRecovery={(run) => setRecovery(run)}
        onSaved={invalidate}
        state={dialogState}
      />
      <RunFeedbackSheet
        expectedTargetId={account?.id ?? ''}
        expectedTargetType='account'
        onOpenChange={(open) => !open && setRecovery(null)}
        open={recovery != null}
        run={recovery}
      />
    </SectionPageLayout>
  )
}
