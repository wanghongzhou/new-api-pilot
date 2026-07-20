import {
  Archive03Icon,
  Delete02Icon,
  Edit03Icon,
  MoreVerticalIcon,
  Refresh01Icon,
  RotateClockwiseIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { QuotaAmount } from '@/components/data/quota-amount'
import { Badge } from '@/components/ui/badge'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import type {
  AccountListItem,
  AccountManagedStatus,
  AccountRemoteState,
} from '../types'

export type AccountAction =
  | 'edit'
  | 'refresh'
  | 'archive'
  | 'restore'
  | 'delete'

const actionIcons = {
  archive: Archive03Icon,
  delete: Delete02Icon,
  edit: Edit03Icon,
  refresh: Refresh01Icon,
  restore: RotateClockwiseIcon,
} as const

export function RemoteStateBadge({ state }: { state: AccountRemoteState }) {
  const { t } = useTranslation()
  let variant: 'destructive' | 'success' | 'warning' = 'destructive'
  if (state === 'normal') variant = 'success'
  else if (state === 'missing') variant = 'warning'
  return (
    <Badge variant={variant}>
      {t(dynamicI18nKey('account', `account.remoteState.${state}`))}
    </Badge>
  )
}

export function ManagedStatusBadge({
  status,
}: {
  status: AccountManagedStatus
}) {
  const { t } = useTranslation()
  return (
    <Badge variant={status === 'active' ? 'success' : 'neutral'}>
      {t(dynamicI18nKey('account', `account.managedStatus.${status}`))}
    </Badge>
  )
}

export function RemoteStatusBadge({ status }: { status: number }) {
  const { t } = useTranslation()
  return (
    <Badge variant={status === 1 ? 'success' : 'destructive'}>
      {t(
        dynamicI18nKey(
          'account',
          status === 1
            ? 'account.remoteStatus.enabled'
            : 'account.remoteStatus.disabled'
        )
      )}
    </Badge>
  )
}

export function AccountActions({
  account,
  onAction,
}: {
  account: AccountListItem
  onAction: (action: AccountAction, account: AccountListItem) => void
}) {
  const { t } = useTranslation()
  const actions: AccountAction[] = [
    'edit',
    'refresh',
    account.managed_status === 'active' ? 'archive' : 'restore',
    'delete',
  ]
  return (
    <details className='relative'>
      <summary
        aria-label={t('account.actions.open')}
        className='hover:bg-muted focus-visible:ring-ring flex size-10 list-none items-center justify-center rounded-md outline-none focus-visible:ring-2 [&::-webkit-details-marker]:hidden'
      >
        <HugeiconsIcon icon={MoreVerticalIcon} strokeWidth={2} />
      </summary>
      <div className='bg-popover absolute top-11 right-0 z-30 grid min-w-48 rounded-md border p-1 shadow-lg'>
        {actions.map((action) => (
          <button
            className={
              action === 'archive' || action === 'delete'
                ? 'text-destructive hover:bg-destructive/10 flex min-h-10 items-center gap-2 rounded-sm px-3 text-left text-sm'
                : 'hover:bg-muted flex min-h-10 items-center gap-2 rounded-sm px-3 text-left text-sm'
            }
            key={action}
            onClick={(event) => {
              event.currentTarget.closest('details')?.removeAttribute('open')
              onAction(action, account)
            }}
            type='button'
          >
            <HugeiconsIcon icon={actionIcons[action]} strokeWidth={2} />
            {t(dynamicI18nKey('account', `account.actions.${action}`))}
          </button>
        ))}
      </div>
    </details>
  )
}

export function AccountCard({
  account,
  isAdmin,
  onAction,
}: {
  account: AccountListItem
  isAdmin: boolean
  onAction: (action: AccountAction, account: AccountListItem) => void
}) {
  const { t } = useTranslation()
  return (
    <article
      className={
        account.managed_status === 'archived'
          ? 'border-border bg-muted/25 grid gap-4 rounded-lg border p-4'
          : 'border-border bg-card grid gap-4 rounded-lg border p-4'
      }
    >
      <div className='flex min-w-0 items-start justify-between gap-2'>
        <div className='min-w-0'>
          <Link
            className='block truncate font-semibold hover:underline'
            params={{ accountId: account.id }}
            to='/accounts/$accountId'
          >
            {account.username}
          </Link>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('account.remoteUserIdValue', { id: account.remote_user_id })}
          </p>
        </div>
        {isAdmin && <AccountActions account={account} onAction={onAction} />}
      </div>
      <div className='flex flex-wrap gap-2'>
        <RemoteStateBadge state={account.remote_state} />
        <ManagedStatusBadge status={account.managed_status} />
        <RemoteStatusBadge status={account.remote_status} />
      </div>
      <dl className='grid grid-cols-2 gap-3 text-sm'>
        <div>
          <dt className='text-muted-foreground text-xs'>{t('account.site')}</dt>
          <dd className='truncate'>{account.site_name}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('account.customer')}
          </dt>
          <dd className='truncate'>{account.customer_name}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('account.remoteGroup')}
          </dt>
          <dd>{account.remote_group || '-'}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('account.todayRequests')}
          </dt>
          <dd>
            <MetricValue compact value={account.today.request_count} />
          </dd>
        </div>
      </dl>
      <QuotaAmount quota={account.today.quota} rate={account.rate} />
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <DataStatusBadge status={account.today.data_status} />
        <DataFreshness
          labelKey='account.asOf'
          timestamp={account.today.as_of}
        />
      </div>
    </article>
  )
}
