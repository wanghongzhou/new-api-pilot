import {
  ArrowRight01Icon,
  Chart01Icon,
  Copy01Icon,
  CpuIcon,
  Database01Icon,
  RamMemoryIcon,
  ServerStack01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { MetricValue } from '@/components/data/metric-value'
import { QuotaAmount } from '@/components/data/quota-amount'
import { SiteStatusBadges } from '@/components/data/site-status-badges'
import { Badge } from '@/components/ui/badge'
import { buildStatisticsSearch } from '@/features/statistics/search'
import { fromUnixSeconds } from '@/lib/dayjs'
import { cn } from '@/lib/utils'

import { formatLatencySeconds, siteResourceColor } from '../site-card-metrics'
import type { SiteListItem } from '../types'
import { SiteActions, type SiteAction } from './site-actions'

function PercentValue({ value }: { value: number | null }) {
  return <span>{`${(value ?? 0).toFixed(1)}%`}</span>
}

function ResourceChip({
  color,
  icon,
  label,
  value,
}: {
  color?: string
  icon: typeof ServerStack01Icon
  label: string
  value: React.ReactNode
}) {
  return (
    <div className='bg-muted/35 grid min-w-0 place-items-center gap-1 rounded-lg px-3 py-2 text-center'>
      <div className='text-muted-foreground flex min-w-0 items-center justify-center gap-1 text-[11px]'>
        <HugeiconsIcon icon={icon} size={13} strokeWidth={2} />
        <span className='truncate'>{label}</span>
      </div>
      <div
        className='text-foreground text-sm leading-none font-semibold tabular-nums'
        style={color == null ? undefined : { color }}
      >
        {value}
      </div>
    </div>
  )
}

function MetricCell({
  children,
  label,
  tone = 'default',
}: {
  children: React.ReactNode
  label: string
  tone?: 'default' | 'success'
}) {
  return (
    <div className='min-w-0 text-center'>
      <p className='text-muted-foreground truncate text-xs'>{label}</p>
      <div
        className={cn(
          'text-foreground mt-1 min-w-0 text-base leading-none font-semibold tabular-nums',
          tone === 'success' && 'text-success'
        )}
      >
        {children}
      </div>
    </div>
  )
}

function CompletenessProgress({
  label,
  value,
}: {
  label: string
  value: number
}) {
  const percent = Math.max(0, Math.min(100, value * 100))
  return (
    <div className='grid gap-1.5'>
      <div className='flex items-center justify-between gap-3'>
        <p className='text-muted-foreground text-xs'>{label}</p>
        <p className='text-muted-foreground text-xs font-semibold tabular-nums'>
          {percent.toFixed(0)}%
        </p>
      </div>
      <div
        aria-label={label}
        aria-valuemax={100}
        aria-valuemin={0}
        aria-valuenow={percent}
        className='bg-muted h-1.5 overflow-hidden rounded-full'
        role='progressbar'
      >
        <div
          className='from-primary to-success h-full rounded-full bg-gradient-to-r transition-[width]'
          style={{ width: `${percent}%` }}
        />
      </div>
    </div>
  )
}

function UpdatedAtLine({
  expired,
  timestamp,
}: {
  expired: boolean
  timestamp: number | null
}) {
  const { t } = useTranslation()
  const exact =
    timestamp == null
      ? null
      : fromUnixSeconds(timestamp).format('YYYY-MM-DD HH:mm:ss')

  return (
    <div className='text-muted-foreground flex min-w-0 items-center gap-2 text-xs'>
      <span className='bg-success size-1.5 shrink-0 rounded-full' />
      <span className='truncate'>
        {exact == null
          ? t('data.noUpdateTime')
          : t('site.currentUpdatedAt', { time: exact })}
      </span>
      {expired && (
        <Badge className='shrink-0' variant='destructive'>
          {t('data.stale')}
        </Badge>
      )}
    </div>
  )
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
  const performanceAvailable = site.performance.data_status === 'complete'

  return (
    <article
      data-slot='site-card'
      className={cn(
        'text-card-foreground flex min-w-0 flex-col gap-3 rounded-lg border bg-(--data-table-card-bg,var(--table-row)) px-3 py-2.5 transition-[background-color,border-color] duration-150',
        site.management_status === 'disabled' && 'saturate-50 opacity-75'
      )}
    >
      <div className='grid min-w-0 gap-3'>
        <header className='flex min-w-0 items-start justify-between gap-3'>
          <div className='min-w-0'>
            <div className='flex min-w-0 items-center gap-1.5'>
              <span className='text-foreground min-w-0 truncate text-base leading-tight font-semibold'>
                {site.name}
              </span>
              {site.online_status === 'online' && (
                <span className='bg-success size-1.5 shrink-0 rounded-full' />
              )}
            </div>
            <div className='mt-1 flex min-w-0 items-center gap-1.5'>
              <p
                className='text-muted-foreground min-w-0 truncate font-mono text-xs'
                title={site.base_url}
              >
                {site.base_url}
              </p>
              <button
                className='text-muted-foreground hover:text-foreground shrink-0 transition-colors'
                onClick={() =>
                  void navigator.clipboard?.writeText(site.base_url)
                }
                title={site.base_url}
                type='button'
              >
                <HugeiconsIcon icon={Copy01Icon} size={14} strokeWidth={2} />
              </button>
            </div>
          </div>
          {isAdmin && <SiteActions onAction={onAction} site={site} />}
        </header>

        <SiteStatusBadges site={site} />

        <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
          <ResourceChip
            icon={ServerStack01Icon}
            label={t('site.instances')}
            value={`${site.resource.online_instance_count ?? 0}/${
              site.resource.instance_count ?? 0
            }`}
          />
          <ResourceChip
            color={siteResourceColor(site.resource.cpu_max_percent)}
            icon={CpuIcon}
            label={t('metric.cpu')}
            value={<PercentValue value={site.resource.cpu_max_percent} />}
          />
          <ResourceChip
            color={siteResourceColor(site.resource.memory_max_percent)}
            icon={RamMemoryIcon}
            label={t('metric.memory')}
            value={<PercentValue value={site.resource.memory_max_percent} />}
          />
          <ResourceChip
            color={siteResourceColor(site.resource.disk_max_used_percent)}
            icon={Database01Icon}
            label={t('metric.disk')}
            value={<PercentValue value={site.resource.disk_max_used_percent} />}
          />
        </div>
      </div>

      <section className='grid gap-3'>
        <div className='grid grid-cols-2 gap-x-5 gap-y-4 sm:grid-cols-3'>
          <MetricCell label={t('site.todayRequests')}>
            <MetricValue
              compact
              nullLabel='0'
              value={site.today.request_count}
            />
          </MetricCell>
          <MetricCell label={t('site.todayQuota')}>
            <QuotaAmount
              className='justify-center'
              emphasizeAmount
              inline
              nullLabel='0'
              quota={site.today.quota}
              rate={site.rate}
              showQuota={false}
            />
          </MetricCell>
          <MetricCell label={t('metric.token')}>
            <MetricValue compact nullLabel='0' value={site.today.token_used} />
          </MetricCell>
          <MetricCell label={t('site.activeUsers')}>
            <MetricValue
              compact
              nullLabel='0'
              value={site.today.active_users}
            />
          </MetricCell>
          <MetricCell label={t('site.averageRpm')}>
            <MetricValue compact nullLabel='0' value={site.today.avg_rpm} />
          </MetricCell>
          <MetricCell label={t('site.averageTpm')}>
            <MetricValue compact nullLabel='0' value={site.today.avg_tpm} />
          </MetricCell>
        </div>
      </section>

      <section className='grid gap-3'>
        <div className='grid grid-cols-2 gap-x-5 gap-y-4 sm:grid-cols-3'>
          <MetricCell
            label={t('site.performance.successRate')}
            tone={performanceAvailable ? 'success' : 'default'}
          >
            {`${(site.performance.success_rate * 100).toFixed(2)}%`}
          </MetricCell>
          <MetricCell label={t('site.performance.avgLatency')}>
            {t('site.performance.latencyValue', {
              value: formatLatencySeconds(site.performance.avg_latency_ms),
            })}
          </MetricCell>
          <MetricCell label={t('site.performance.avgTps')}>
            {site.performance.avg_tps.toFixed(1)}
          </MetricCell>
        </div>
        <CompletenessProgress
          label={t('site.completeness')}
          value={site.completeness_rate}
        />
      </section>

      <footer className='flex items-center justify-between gap-3 pt-0.5'>
        <UpdatedAtLine
          expired={site.realtime.expired}
          timestamp={site.realtime.updated_at}
        />
        <div className='flex shrink-0 items-center justify-end gap-1'>
          <Link
            aria-label={t('site.actions.stats')}
            className='text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-ring flex size-8 items-center justify-center rounded-md transition-colors outline-none focus-visible:ring-2'
            params={{ siteId: site.id }}
            search={buildStatisticsSearch({})}
            title={t('site.actions.stats')}
            to='/sites/$siteId/stats'
          >
            <HugeiconsIcon icon={Chart01Icon} size={17} strokeWidth={2} />
          </Link>
          <Link
            aria-label={t('site.instanceStatus')}
            className='text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-ring flex size-8 items-center justify-center rounded-md transition-colors outline-none focus-visible:ring-2'
            params={{ siteId: site.id }}
            title={t('site.instanceStatus')}
            to='/sites/$siteId/status'
          >
            <HugeiconsIcon icon={ServerStack01Icon} size={17} strokeWidth={2} />
          </Link>
          <Link
            aria-label={t('site.viewDetails')}
            className='text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-ring flex size-8 items-center justify-center rounded-md transition-colors outline-none focus-visible:ring-2'
            params={{ siteId: site.id }}
            title={t('site.viewDetails')}
            to='/sites/$siteId'
          >
            <HugeiconsIcon icon={ArrowRight01Icon} size={17} strokeWidth={2} />
          </Link>
        </div>
      </footer>
    </article>
  )
}
