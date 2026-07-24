import { useTranslation } from 'react-i18next'

import { FacetedFilter } from '@/components/data/faceted-filter'
import { FilterPanel } from '@/components/data/filter-panel'
import { SelectControl as Select } from '@/components/ui/select-control'
import type { SiteListItem } from '@/features/sites/types'
import { isIdString } from '@/lib/api-types'
import { hasFilterChanges } from '@/lib/filter-state'

import { alertLevels, alertStatuses, alertTargetTypes } from '../constants'
import type { AlertSearch } from '../types'
import { AlertDateTimeRangePicker } from './alert-date-time-range-picker'
import {
  alertLevelText,
  alertStatusText,
  alertTargetTypeText,
} from './alert-ui'

type AlertFilterValue = Pick<
  AlertSearch,
  'end' | 'level' | 'siteId' | 'start' | 'status' | 'targetType'
>

const resetValue: AlertFilterValue = {
  end: undefined,
  level: [],
  siteId: undefined,
  start: undefined,
  status: [],
  targetType: [],
}

export function AlertFilters({
  onApply,
  sites,
  value,
}: {
  onApply: (value: AlertFilterValue) => void
  sites: SiteListItem[]
  value: AlertFilterValue
}) {
  const { t } = useTranslation()
  const apply = (next: AlertFilterValue) => onApply(next)

  return (
    <FilterPanel
      description={t('alerts.filters.description')}
      hasActiveFilters={hasFilterChanges(value, resetValue, [
        'end',
        'level',
        'siteId',
        'start',
        'status',
        'targetType',
      ])}
      onReset={() => apply({ ...resetValue })}
      title={t('alerts.filters.title')}
    >
      <div className='flex flex-wrap items-center gap-2'>
        <FacetedFilter
          clearLabel={t('common.clearFilters')}
          onChange={(status) =>
            apply({
              ...value,
              status: alertStatuses.includes(
                status as (typeof alertStatuses)[number]
              )
                ? [status as (typeof alertStatuses)[number]]
                : [],
            })
          }
          options={alertStatuses.map((status) => ({
            label: alertStatusText(t, status),
            value: status,
          }))}
          title={t('alerts.table.status')}
          value={value.status[0] ?? ''}
        />
        <FacetedFilter
          clearLabel={t('common.clearFilters')}
          onChange={(level) =>
            apply({
              ...value,
              level: alertLevels.includes(level as (typeof alertLevels)[number])
                ? [level as (typeof alertLevels)[number]]
                : [],
            })
          }
          options={alertLevels.map((level) => ({
            label: alertLevelText(t, level),
            value: level,
          }))}
          title={t('alerts.table.level')}
          value={value.level[0] ?? ''}
        />
        <FacetedFilter
          clearLabel={t('common.clearFilters')}
          onChange={(targetType) =>
            apply({
              ...value,
              targetType: alertTargetTypes.includes(
                targetType as (typeof alertTargetTypes)[number]
              )
                ? [targetType as (typeof alertTargetTypes)[number]]
                : [],
            })
          }
          options={alertTargetTypes.map((targetType) => ({
            label: alertTargetTypeText(t, targetType),
            value: targetType,
          }))}
          title={t('alerts.filters.targetType')}
          value={value.targetType[0] ?? ''}
        />
        <div className='grid gap-1.5 text-sm'>
          <Select
            aria-label={t('alerts.table.site')}
            onChange={(event) =>
              apply({
                ...value,
                siteId: isIdString(event.target.value)
                  ? event.target.value
                  : undefined,
              })
            }
            value={value.siteId ?? ''}
          >
            <option value=''>{t('alerts.filters.allSites')}</option>
            {sites.map((site) => (
              <option key={site.id} value={site.id}>
                {site.name}
              </option>
            ))}
          </Select>
        </div>
        <AlertDateTimeRangePicker
          end={value.end}
          onChange={(range) => apply({ ...value, ...range })}
          start={value.start}
        />
      </div>
    </FilterPanel>
  )
}
