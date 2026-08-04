import {
  Analytics01Icon,
  LaptopPerformanceIcon,
  Money03Icon,
  RankingIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { buildFinancialOperationsSearch } from '@/features/financial-operations/search'
import { buildPerformanceHistorySearch } from '@/features/performance-history/search'
import { buildRankingSearch } from '@/features/rankings/search'
import { buildStatisticsSearch } from '@/features/statistics/search'
import { cn } from '@/lib/utils'

export type OperationsAnalyticsSection =
  | 'financial'
  | 'performance'
  | 'rankings'
  | 'statistics'

type AnalyticsIcon = typeof Money03Icon

const sections: ReadonlyArray<{
  icon: AnalyticsIcon
  value: OperationsAnalyticsSection
}> = [
  {
    icon: Money03Icon,
    value: 'financial',
  },
  {
    icon: Analytics01Icon,
    value: 'statistics',
  },
  {
    icon: RankingIcon,
    value: 'rankings',
  },
  {
    icon: LaptopPerformanceIcon,
    value: 'performance',
  },
]

function SectionLink({
  active,
  icon,
  label,
  section,
  siteId,
}: {
  active: boolean
  icon: AnalyticsIcon
  label: string
  section: OperationsAnalyticsSection
  siteId: string
}) {
  const className = cn(
    'flex min-h-11 min-w-0 items-center gap-2 rounded-lg px-3 text-sm font-medium transition-colors sm:shrink-0',
    active
      ? 'bg-background text-foreground shadow-sm ring-1 ring-foreground/10'
      : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
  )
  const content = (
    <>
      <HugeiconsIcon icon={icon} size={17} strokeWidth={2} />
      <span>{label}</span>
    </>
  )

  if (section === 'financial') {
    return (
      <Link
        aria-current={active ? 'page' : undefined}
        className={className}
        params={{ siteId }}
        search={buildFinancialOperationsSearch({})}
        to='/sites/$siteId/financial-operations'
      >
        {content}
      </Link>
    )
  }
  if (section === 'statistics') {
    return (
      <Link
        aria-current={active ? 'page' : undefined}
        className={className}
        params={{ siteId }}
        search={buildStatisticsSearch({})}
        to='/sites/$siteId/stats'
      >
        {content}
      </Link>
    )
  }
  if (section === 'rankings') {
    return (
      <Link
        aria-current={active ? 'page' : undefined}
        className={className}
        params={{ siteId }}
        search={buildRankingSearch({})}
        to='/sites/$siteId/rankings'
      >
        {content}
      </Link>
    )
  }
  return (
    <Link
      aria-current={active ? 'page' : undefined}
      className={className}
      params={{ siteId }}
      search={buildPerformanceHistorySearch({})}
      to='/sites/$siteId/performance-history'
    >
      {content}
    </Link>
  )
}

export function OperationsAnalyticsNavigation({
  active,
  siteId,
}: {
  active: OperationsAnalyticsSection
  siteId: string
}) {
  const { t } = useTranslation()
  const labels: Record<OperationsAnalyticsSection, string> = {
    financial: t('operationsAnalytics.navigation.financial'),
    performance: t('operationsAnalytics.navigation.performance'),
    rankings: t('operationsAnalytics.navigation.rankings'),
    statistics: t('operationsAnalytics.navigation.statistics'),
  }
  return (
    <nav
      aria-label={t('operationsAnalytics.navigation.label')}
      className='bg-muted/50 min-w-0 rounded-xl p-1'
    >
      <div className='grid grid-cols-2 gap-1 sm:flex sm:min-w-max'>
        {sections.map((section) => (
          <SectionLink
            active={section.value === active}
            icon={section.icon}
            key={section.value}
            label={labels[section.value]}
            section={section.value}
            siteId={siteId}
          />
        ))}
      </div>
    </nav>
  )
}

export function OperationsViewPurpose({
  badges,
  description,
  icon,
  notice,
  title,
}: {
  badges?: ReactNode
  description: ReactNode
  icon: AnalyticsIcon
  notice?: ReactNode
  title: ReactNode
}) {
  return (
    <section className='border-border bg-muted/20 flex items-start gap-3 rounded-xl border p-4'>
      <span className='bg-background text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg border'>
        <HugeiconsIcon icon={icon} size={18} strokeWidth={2} />
      </span>
      <div className='min-w-0 flex-1'>
        <div className='flex flex-wrap items-center gap-2'>
          <h2 className='font-medium'>{title}</h2>
          {badges}
        </div>
        <p className='text-muted-foreground mt-1 text-sm'>{description}</p>
        {notice != null && (
          <p className='text-muted-foreground mt-1 text-xs'>{notice}</p>
        )}
      </div>
    </section>
  )
}
