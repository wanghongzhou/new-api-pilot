import {
  Archive03Icon,
  Chart01Icon,
  Delete02Icon,
  Edit03Icon,
  MoreVerticalIcon,
  Refresh01Icon,
  UserGroupIcon,
  ViewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
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
import { calculateCrossSiteQuotaAmount, formatDecimal } from '@/lib/amount'
import { formatDecimalDisplayValue } from '@/lib/display-value'
import { cn } from '@/lib/utils'

import type { CustomerListItem, CustomerStatus } from '../types'

export type CustomerAction = 'edit' | 'disable' | 'enable' | 'delete'

const customerActionIcons = {
  delete: Delete02Icon,
  disable: Archive03Icon,
  edit: Edit03Icon,
  enable: Refresh01Icon,
} as const

export function CustomerStatusBadge({ status }: { status: CustomerStatus }) {
  const { t } = useTranslation()
  let variant: 'destructive' | 'neutral' | 'primary' | 'success' = 'neutral'
  if (status === 'using') variant = 'success'
  else if (status === 'signing') variant = 'primary'
  else if (status === 'disabled') variant = 'destructive'
  return (
    <Badge variant={variant}>
      {t(dynamicI18nKey('customer', `customer.status.${status}`))}
    </Badge>
  )
}

export function CustomerQuotaAmount({
  customer,
  compact = false,
}: {
  customer: CustomerListItem
  compact?: boolean
}) {
  const { t } = useTranslation()
  const amount = useMemo(
    () =>
      calculateCrossSiteQuotaAmount(
        customer.today.site_breakdown.map((site) => ({
          quota: site.quota,
          rate: {
            quota_per_unit: site.quota_per_unit,
            source: site.rate_source,
            updated_at: site.rate_updated_at,
            usd_exchange_rate: site.usd_exchange_rate,
          },
          siteId: site.site_id,
        }))
      ),
    [customer.today.site_breakdown]
  )
  return (
    <div className={cn('grid gap-0.5', compact && 'justify-items-center')}>
      <span>
        <MetricValue compact nullLabel='0' value={customer.today.quota} />
        <span className='text-muted-foreground ml-1 text-xs'>
          {t('metric.quota')}
        </span>
      </span>
      {amount.status === 'available' ? (
        <span className='text-muted-foreground text-xs'>
          {t('amount.summary', {
            cny: formatDecimal(amount.amountCny),
            usd: formatDecimal(amount.amountUsd),
          })}
        </span>
      ) : (
        <span className='text-warning-foreground text-xs'>
          {t(
            dynamicI18nKey(
              'customer',
              amount.status === 'partial_rate_unavailable'
                ? 'amount.partialRateUnavailable'
                : 'amount.rateUnavailable'
            )
          )}
        </span>
      )}
    </div>
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

export function CustomerActions({
  customer,
  onAction,
}: {
  customer: CustomerListItem
  onAction: (action: CustomerAction, customer: CustomerListItem) => void
}) {
  const { t } = useTranslation()
  const actions: CustomerAction[] =
    customer.status === 'disabled' ? ['enable'] : ['edit', 'disable', 'delete']
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            aria-label={t('customer.actions.open')}
            className='size-10 sm:size-8'
            size='icon'
            title={t('customer.actions.open')}
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
            onClick={() => onAction(action, customer)}
            variant={
              action === 'delete' || action === 'disable'
                ? 'destructive'
                : 'default'
            }
          >
            <HugeiconsIcon icon={customerActionIcons[action]} strokeWidth={2} />
            {t(dynamicI18nKey('customer', `customer.actions.${action}`))}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function CustomerCard({
  customer,
  isAdmin,
  onAction,
  onOpenAccounts,
}: {
  customer: CustomerListItem
  isAdmin: boolean
  onAction: (action: CustomerAction, customer: CustomerListItem) => void
  onOpenAccounts: (customerId: string) => void
}) {
  const { t } = useTranslation()
  return (
    <article
      className={cn(
        'text-card-foreground flex min-w-0 flex-col gap-3 rounded-lg border bg-(--data-table-card-bg,var(--table-row)) px-3 py-2.5 transition-[background-color,border-color] duration-150',
        customer.status === 'disabled' && 'saturate-50 opacity-75'
      )}
      data-slot='customer-card'
    >
      <div className='flex min-w-0 items-start justify-between gap-2'>
        <div className='min-w-0'>
          <Link
            className='block truncate text-base leading-tight font-semibold hover:underline'
            params={{ customerId: customer.id }}
            to='/customers/$customerId'
          >
            {customer.name}
          </Link>
          <p className='text-muted-foreground mt-1 truncate text-xs'>
            {customer.contact || '-'}
          </p>
        </div>
        {isAdmin && <CustomerActions customer={customer} onAction={onAction} />}
      </div>
      <div className='flex flex-wrap gap-2'>
        <CustomerStatusBadge status={customer.status} />
      </div>
      <div className='grid grid-cols-2 gap-2 text-sm'>
        <dl className='bg-muted/35 min-w-0 rounded-md px-2.5 py-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('customer.contractAmount')}
          </dt>
          <dd
            className='mt-1 truncate font-medium tabular-nums'
            title={customer.contract_amount}
          >
            {formatDecimalDisplayValue(customer.contract_amount)}
          </dd>
        </dl>
        <dl className='bg-muted/35 min-w-0 rounded-md px-2.5 py-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('customer.paymentAmount')}
          </dt>
          <dd
            className='mt-1 truncate font-medium tabular-nums'
            title={customer.payment_amount}
          >
            {formatDecimalDisplayValue(customer.payment_amount)}
          </dd>
        </dl>
        <dl className='bg-muted/35 min-w-0 rounded-md px-2.5 py-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('customer.accounts')}
          </dt>
          <dd className='mt-1 font-medium tabular-nums'>
            {customer.active_account_count}/{customer.account_count}
          </dd>
        </dl>
        <dl className='bg-muted/35 min-w-0 rounded-md px-2.5 py-2'>
          <dt className='text-muted-foreground text-xs'>
            {t('customer.sites')}
          </dt>
          <dd className='mt-1 font-medium tabular-nums'>
            {customer.site_count}
          </dd>
        </dl>
      </div>
      <section className='grid gap-3'>
        <div className='grid grid-cols-2 gap-x-5 gap-y-4'>
          <MetricCell label={t('site.dashboard.totalQuota')}>
            <CustomerQuotaAmount compact customer={customer} />
          </MetricCell>
          <MetricCell label={t('site.dashboard.totalTokens')}>
            <MetricValue
              compact
              nullLabel='0'
              value={customer.today.token_used}
            />
          </MetricCell>
        </div>
        <div className='grid grid-cols-3 gap-x-5 gap-y-4'>
          <MetricCell label={t('site.dashboard.totalCount')}>
            <MetricValue
              compact
              nullLabel='0'
              value={customer.today.request_count}
            />
          </MetricCell>
          <MetricCell label={t('site.averageRpm')}>
            {formatAverageRate(customer.today.avg_rpm)}
          </MetricCell>
          <MetricCell label={t('site.averageTpm')}>
            {formatAverageRate(customer.today.avg_tpm)}
          </MetricCell>
        </div>
      </section>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex items-center gap-2'>
          <DataStatusBadge status={customer.today.data_status} />
          <span className='text-muted-foreground text-xs'>
            {t('customer.activeAccounts')}:{' '}
            <MetricValue
              compact
              nullLabel='0'
              value={customer.today.active_users}
            />
          </span>
        </div>
        <DataFreshness
          labelKey='customer.asOf'
          timestamp={customer.today.as_of}
        />
      </div>
      <footer className='border-border/70 grid grid-cols-3 gap-1 border-t pt-2'>
        <Link
          className='hover:bg-muted inline-flex min-h-9 min-w-0 items-center justify-center gap-1 rounded-md px-1.5 text-xs font-medium'
          params={{ customerId: customer.id }}
          search={buildStatisticsSearch({})}
          to='/customers/$customerId/stats'
        >
          <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
          {t('customer.actions.stats')}
        </Link>
        <button
          className='hover:bg-muted inline-flex min-h-9 min-w-0 items-center justify-center gap-1 rounded-md px-1.5 text-xs font-medium'
          onClick={() => onOpenAccounts(customer.id)}
          type='button'
        >
          <HugeiconsIcon icon={UserGroupIcon} strokeWidth={2} />
          {t('customer.actions.accounts')}
        </button>
        <Link
          className='hover:bg-muted inline-flex min-h-9 min-w-0 items-center justify-center gap-1 rounded-md px-1.5 text-xs font-medium'
          params={{ customerId: customer.id }}
          to='/customers/$customerId'
        >
          <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
          {t('customer.actions.detail')}
        </Link>
      </footer>
    </article>
  )
}
