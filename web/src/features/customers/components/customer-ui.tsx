import {
  Archive03Icon,
  Delete02Icon,
  Edit03Icon,
  MoreVerticalIcon,
  Refresh01Icon,
  ViewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataFreshness } from '@/components/data/data-freshness'
import { DataStatusBadge } from '@/components/data/data-status'
import { MetricValue } from '@/components/data/metric-value'
import { Badge } from '@/components/ui/badge'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { calculateCrossSiteQuotaAmount, formatDecimal } from '@/lib/amount'

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
}: {
  customer: CustomerListItem
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
    <div className='grid gap-0.5'>
      <span>
        <MetricValue compact value={customer.today.quota} />
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
    <details className='relative'>
      <summary
        aria-label={t('customer.actions.open')}
        className='hover:bg-muted focus-visible:ring-ring flex size-10 list-none items-center justify-center rounded-md outline-none focus-visible:ring-2 [&::-webkit-details-marker]:hidden'
      >
        <HugeiconsIcon icon={MoreVerticalIcon} strokeWidth={2} />
      </summary>
      <div className='bg-popover absolute top-11 right-0 z-30 grid min-w-48 rounded-md border p-1 shadow-lg'>
        {actions.map((action) => (
          <button
            className={
              action === 'delete' || action === 'disable'
                ? 'text-destructive hover:bg-destructive/10 flex min-h-10 items-center gap-2 rounded-sm px-3 text-left text-sm'
                : 'hover:bg-muted flex min-h-10 items-center gap-2 rounded-sm px-3 text-left text-sm'
            }
            key={action}
            onClick={(event) => {
              event.currentTarget.closest('details')?.removeAttribute('open')
              onAction(action, customer)
            }}
            type='button'
          >
            <HugeiconsIcon icon={customerActionIcons[action]} strokeWidth={2} />
            {t(dynamicI18nKey('customer', `customer.actions.${action}`))}
          </button>
        ))}
      </div>
    </details>
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
      className={
        customer.status === 'disabled'
          ? 'border-border bg-muted/25 grid gap-4 rounded-lg border p-4'
          : 'border-border bg-card grid gap-4 rounded-lg border p-4'
      }
    >
      <div className='flex min-w-0 items-start justify-between gap-2'>
        <div className='min-w-0'>
          <Link
            className='truncate font-semibold hover:underline'
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
        <DataStatusBadge status={customer.today.data_status} />
      </div>
      <dl className='grid grid-cols-2 gap-3 text-sm'>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('customer.accounts')}
          </dt>
          <dd>
            {customer.active_account_count}/{customer.account_count}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('customer.sites')}
          </dt>
          <dd>{customer.site_count}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('customer.todayRequests')}
          </dt>
          <dd>
            <MetricValue compact value={customer.today.request_count} />
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('customer.activeAccounts')}
          </dt>
          <dd>
            <MetricValue compact value={customer.today.active_users} />
          </dd>
        </div>
      </dl>
      <CustomerQuotaAmount customer={customer} />
      <DataFreshness
        labelKey='customer.asOf'
        timestamp={customer.today.as_of}
      />
      <button
        className='hover:bg-muted inline-flex min-h-10 items-center justify-center gap-2 rounded-md border px-3 text-sm font-medium'
        onClick={() => onOpenAccounts(customer.id)}
        type='button'
      >
        <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
        {t('customer.actions.accounts')}
      </button>
    </article>
  )
}
