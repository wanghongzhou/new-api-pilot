import { Alert02Icon, CheckmarkCircle02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import type { Completeness } from '@/features/sites/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { fromUnixSeconds } from '@/lib/dayjs'
import { translateMessageRef } from '@/lib/message-ref'

import { DataStatusBadge } from './data-status'

export function CompletenessAlert({
  completeness,
}: {
  completeness: Completeness
}) {
  const { t } = useTranslation()
  const complete = completeness.data_status === 'complete'
  const percentage = Math.round(completeness.completeness_rate * 1000) / 10
  return (
    <section
      className={
        complete
          ? 'border-success/30 bg-success/5 rounded-lg border p-4'
          : 'border-warning/35 bg-warning/8 rounded-lg border p-4'
      }
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h2 className='flex items-center gap-2 font-medium'>
          <HugeiconsIcon
            icon={complete ? CheckmarkCircle02Icon : Alert02Icon}
            strokeWidth={2}
          />
          {t('completeness.title')}
        </h2>
        <DataStatusBadge status={completeness.data_status} />
      </div>
      <p className='mt-2 text-sm'>
        {t('completeness.units', {
          complete: completeness.complete_unit_count,
          expected: completeness.expected_unit_count,
          rate: percentage,
        })}
      </p>
      <dl className='mt-3 grid gap-2 text-sm sm:grid-cols-2'>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('completeness.unitTypeLabel')}
          </dt>
          <dd>
            {t(
              dynamicI18nKey(
                'data',
                `completeness.unitType.${completeness.unit_type}`
              )
            )}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('completeness.siteCoverageLabel')}
          </dt>
          <dd>
            {t('completeness.siteCoverage', {
              complete: completeness.complete_site_count,
              expected: completeness.expected_site_count,
            })}
          </dd>
        </div>
      </dl>
      {completeness.missing_site_ids.length > 0 && (
        <p className='text-muted-foreground mt-2 text-xs'>
          {t('completeness.missingSiteIds', {
            ids: completeness.missing_site_ids.join(', '),
          })}
        </p>
      )}
      {completeness.last_verified_at != null && (
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('completeness.lastVerified', {
            time: fromUnixSeconds(completeness.last_verified_at).format(
              'YYYY-MM-DD HH:mm:ss'
            ),
          })}
        </p>
      )}
      {completeness.missing_ranges.length > 0 && (
        <ul className='mt-3 grid gap-2 text-sm'>
          {completeness.missing_ranges.slice(0, 3).map((range) => (
            <li
              className='border-border rounded-md border p-2'
              key={`${range.site_id}-${range.start_timestamp}`}
            >
              <span className='font-medium'>
                {fromUnixSeconds(range.start_timestamp).format(
                  'YYYY-MM-DD HH:00'
                )}
                {' - '}
                {fromUnixSeconds(range.end_timestamp).format(
                  'YYYY-MM-DD HH:00'
                )}
              </span>
              <span className='text-muted-foreground ml-2'>
                {translateMessageRef(range.reason)}
              </span>
            </li>
          ))}
        </ul>
      )}
      {completeness.missing_ranges_truncated && (
        <p className='text-muted-foreground mt-2 text-xs'>
          {t('completeness.truncated', {
            total: completeness.missing_range_total,
          })}
        </p>
      )}
    </section>
  )
}
