import {
  Archive03Icon,
  Chart01Icon,
  Delete02Icon,
  Edit03Icon,
  MoreVerticalIcon,
  Refresh01Icon,
  RotateClockwiseIcon,
  ViewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { QuotaAmount } from '@/components/data/quota-amount'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { formatAverageRate } from '@/features/sites/site-card-metrics'
import { buildStatisticsSearch } from '@/features/statistics/search'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { cn } from '@/lib/utils'

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
  onAction: (
    action: AccountAction,
    account: AccountListItem,
    trigger: HTMLButtonElement | null
  ) => void
}) {
  const { t } = useTranslation()
  const triggerRef = useRef<HTMLButtonElement>(null)
  const actions: AccountAction[] = [
    'edit',
    'refresh',
    account.managed_status === 'active' ? 'archive' : 'restore',
    'delete',
  ]
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            aria-label={t('account.actions.open')}
            ref={triggerRef}
            className='size-10 sm:size-8'
            size='icon'
            title={t('account.actions.open')}
            variant='ghost'
          />
        }
      >
        <HugeiconsIcon icon={MoreVerticalIcon} strokeWidth={2} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='min-w-48'>
        {actions.map((action) => (
          <DropdownMenuItem
            key={action}
            onClick={() => onAction(action, account, triggerRef.current)}
            variant={
              action === 'archive' || action === 'delete'
                ? 'destructive'
                : 'default'
            }
          >
            <HugeiconsIcon icon={actionIcons[action]} strokeWidth={2} />
            {t(dynamicI18nKey('account', `account.actions.${action}`))}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function AccountCard({
  account,
  isAdmin,
  onAction,
}: {
  account: AccountListItem
  isAdmin: boolean
  onAction: (
    action: AccountAction,
    account: AccountListItem,
    trigger: HTMLButtonElement | null
  ) => void
}) {
  const { t } = useTranslation()
  return (
    <article
      className={cn(
        'text-card-foreground flex min-w-0 flex-col gap-3 rounded-lg border bg-(--data-table-card-bg,var(--table-row)) px-3 py-2.5 transition-[background-color,border-color] duration-150',
        account.managed_status === 'archived' && 'saturate-50 opacity-75'
      )}
      data-slot='account-card'
    >
      <div className='flex min-w-0 items-start justify-between gap-2'>
        <div className='min-w-0'>
          <Link
            className='block truncate text-base leading-tight font-semibold hover:underline'
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
      <div className='grid grid-cols-2 gap-2 text-sm'>
        <dl className='bg-muted/35 min-w-0 rounded-md px-2.5 py-2'>
          <dt className='text-muted-foreground text-xs'>{t('account.site')}</dt>
          <dd className='mt-1 truncate font-medium'>{account.site_name}</dd>
        </dl>
        <dl className='bg-muted/35 min-w-0 rounded-md px-2.5 py-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('account.customer')}
          </dt>
          <dd className='mt-1 truncate font-medium'>{account.customer_name}</dd>
        </dl>
        <dl className='bg-muted/35 col-span-2 min-w-0 rounded-md px-2.5 py-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('account.remoteGroup')}
          </dt>
          <dd className='mt-1 truncate font-medium'>
            {account.remote_group || '-'}
          </dd>
        </dl>
        <dl className='bg-muted/35 min-w-0 rounded-md px-2.5 py-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('account.currentQuota')}
          </dt>
          <dd className='mt-1 font-medium tabular-nums'>
            <MetricValue compact nullLabel='0' value={account.quota} />
          </dd>
        </dl>
        <dl className='bg-muted/35 min-w-0 rounded-md px-2.5 py-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('account.usedQuota')}
          </dt>
          <dd className='mt-1 font-medium tabular-nums'>
            <MetricValue compact nullLabel='0' value={account.used_quota} />
          </dd>
        </dl>
      </div>
      <section className='grid gap-3'>
        <div className='grid grid-cols-2 gap-x-5 gap-y-4'>
          <MetricCell label={t('site.dashboard.todayQuota')}>
            <QuotaAmount
              className='justify-items-center'
              nullLabel='0'
              quota={account.today.quota}
              rate={account.rate}
            />
          </MetricCell>
          <MetricCell label={t('site.dashboard.todayTokens')}>
            <MetricValue
              compact
              nullLabel='0'
              value={account.today.token_used}
            />
          </MetricCell>
        </div>
        <div className='grid grid-cols-3 gap-x-5 gap-y-4'>
          <MetricCell label={t('site.dashboard.todayCount')}>
            <MetricValue
              compact
              nullLabel='0'
              value={account.today.request_count}
            />
          </MetricCell>
          <MetricCell label={t('site.averageRpm')}>
            {formatAverageRate(account.today.avg_rpm)}
          </MetricCell>
          <MetricCell label={t('site.averageTpm')}>
            {formatAverageRate(account.today.avg_tpm)}
          </MetricCell>
        </div>
      </section>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <DataStatusBadge status={account.today.data_status} />
        <DataFreshness
          labelKey='account.asOf'
          timestamp={account.today.as_of}
        />
      </div>
      <footer className='border-border/70 flex flex-wrap items-center gap-1 border-t pt-2'>
        <Link
          className='hover:bg-muted inline-flex min-h-9 items-center gap-1.5 rounded-md px-2.5 text-sm font-medium'
          params={{ accountId: account.id }}
          search={buildStatisticsSearch({})}
          to='/accounts/$accountId/stats'
        >
          <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
          {t('account.actions.stats')}
        </Link>
        <Link
          className='hover:bg-muted ml-auto inline-flex min-h-9 items-center gap-1.5 rounded-md px-2.5 text-sm font-medium'
          params={{ accountId: account.id }}
          to='/accounts/$accountId'
        >
          <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
          {t('account.actions.detail')}
        </Link>
      </footer>
    </article>
  )
}

function MetricCell({
  children,
  label,
}: {
  children: ReactNode
  label: string
}) {
  return (
    <div className='min-w-0 text-center'>
      <p className='text-muted-foreground truncate text-xs'>{label}</p>
      <div className='text-foreground mt-1 min-w-0 text-base leading-none font-semibold tabular-nums'>
        {children}
      </div>
    </div>
  )
}
