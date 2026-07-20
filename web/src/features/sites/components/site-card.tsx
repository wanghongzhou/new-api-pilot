import {
  ArrowUpRight01Icon,
  Chart01Icon,
  ServerStack01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { DataFreshness } from '@/components/data/data-freshness'
import { MetricValue } from '@/components/data/metric-value'
import { QuotaAmount } from '@/components/data/quota-amount'
import { SiteStatusBadges } from '@/components/data/site-status-badges'
import { buildStatisticsSearch } from '@/features/statistics/search'
import { cn } from '@/lib/utils'

import type { SiteListItem } from '../types'
import { SiteActions, type SiteAction } from './site-actions'

function PercentValue({ value }: { value: number | null }) {
  return <span>{`${(value ?? 0).toFixed(1)}%`}</span>
}

export function SiteCard({
  isAdmin,
  onAction,
  site,
}: {
  isAdmin: boolean
  onAction: (action: SiteAction, site: SiteListItem) => void
  site: SiteListItem
}) {
  const { t } = useTranslation()
  return (
    <article
      className={cn(
        'border-border bg-card grid min-w-0 content-between overflow-visible rounded-lg border',
        site.management_status === 'disabled' && 'saturate-50 opacity-75'
      )}
    >
      <div className='grid min-w-0 gap-4 p-4'>
        <div className='flex min-w-0 items-start justify-between gap-3'>
          <div className='min-w-0'>
            <h2 className='truncate font-semibold' title={site.name}>
              {site.name}
            </h2>
            <p
              className='text-muted-foreground mt-0.5 truncate text-xs'
              title={site.base_url}
            >
              {site.base_url}
            </p>
          </div>
          {isAdmin && <SiteActions onAction={onAction} site={site} />}
        </div>

        <SiteStatusBadges site={site} />

        <dl className='grid grid-cols-3 gap-3 text-sm'>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('site.instances')}
            </dt>
            <dd className='font-medium'>
              {site.resource.online_instance_count ?? 0}/
              {site.resource.instance_count ?? 0}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>{t('metric.cpu')}</dt>
            <dd className='font-medium'>
              <PercentValue value={site.resource.cpu_max_percent} />
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('metric.memory')}
            </dt>
            <dd className='font-medium'>
              <PercentValue value={site.resource.memory_max_percent} />
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>{t('metric.rpm')}</dt>
            <dd className='font-medium'>
              <MetricValue compact nullLabel='0' value={site.realtime.rpm} />
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>{t('metric.tpm')}</dt>
            <dd className='font-medium'>
              <MetricValue compact nullLabel='0' value={site.realtime.tpm} />
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('metric.disk')}
            </dt>
            <dd className='font-medium'>
              <PercentValue value={site.resource.disk_max_used_percent} />
            </dd>
          </div>
        </dl>

        <div className='border-border grid grid-cols-2 gap-3 border-t pt-3 text-sm'>
          <div>
            <p className='text-muted-foreground text-xs'>
              {t('site.todayRequests')}
            </p>
            <p className='font-medium'>
              <MetricValue
                compact
                nullLabel='0'
                value={site.today.request_count}
              />
            </p>
          </div>
          <QuotaAmount
            nullLabel='0'
            quota={site.today.quota}
            rate={site.rate}
          />
          <div>
            <p className='text-muted-foreground text-xs'>{t('metric.token')}</p>
            <p className='font-medium'>
              <MetricValue
                compact
                nullLabel='0'
                value={site.today.token_used}
              />
            </p>
          </div>
          <div>
            <p className='text-muted-foreground text-xs'>
              {t('site.activeUsers')}
            </p>
            <p className='font-medium'>
              <MetricValue
                compact
                nullLabel='0'
                value={site.today.active_users}
              />
            </p>
          </div>
          <div>
            <p className='text-muted-foreground text-xs'>
              {t('site.completeness')}
            </p>
            <p className='font-medium'>
              {(site.completeness_rate * 100).toFixed(1)}%
            </p>
          </div>
        </div>

        <DataFreshness
          expired={site.realtime.expired}
          labelKey='site.currentUpdatedAt'
          timestamp={site.realtime.updated_at}
        />
      </div>
      <div className='border-border flex items-center gap-1 border-t p-2'>
        <Link
          className='hover:bg-muted focus-visible:ring-ring flex min-h-10 flex-1 items-center justify-center gap-2 rounded-md px-2 text-sm font-medium outline-none focus-visible:ring-2'
          params={{ siteId: site.id }}
          to='/sites/$siteId'
        >
          <HugeiconsIcon icon={ArrowUpRight01Icon} strokeWidth={2} />
          {t('site.viewDetails')}
        </Link>
        <Link
          aria-label={t('site.actions.stats')}
          className='hover:bg-muted focus-visible:ring-ring flex size-10 items-center justify-center rounded-md outline-none focus-visible:ring-2'
          params={{ siteId: site.id }}
          search={buildStatisticsSearch({})}
          title={t('site.actions.stats')}
          to='/sites/$siteId/stats'
        >
          <HugeiconsIcon icon={Chart01Icon} strokeWidth={2} />
        </Link>
        <Link
          aria-label={t('site.instanceStatus')}
          className='hover:bg-muted focus-visible:ring-ring flex size-10 items-center justify-center rounded-md outline-none focus-visible:ring-2'
          params={{ siteId: site.id }}
          title={t('site.instanceStatus')}
          to='/sites/$siteId/status'
        >
          <HugeiconsIcon icon={ServerStack01Icon} strokeWidth={2} />
        </Link>
      </div>
    </article>
  )
}
