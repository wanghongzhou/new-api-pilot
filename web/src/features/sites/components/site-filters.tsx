import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { FilterPanel } from '@/components/data/filter-panel'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import {
  siteAuthStatuses,
  siteHealthStatuses,
  siteManagementStatuses,
  siteOnlineStatuses,
  siteStatisticsStatuses,
} from '../constants'
import type { SiteSearch } from '../types'

type FilterKey = 'management' | 'online' | 'auth' | 'statistics' | 'health'

const primaryGroups = [
  ['online', siteOnlineStatuses],
  ['auth', siteAuthStatuses],
] as const

const advancedGroups = [
  ['management', siteManagementStatuses],
  ['statistics', siteStatisticsStatuses],
  ['health', siteHealthStatuses],
] as const

type FilterState = Pick<SiteSearch, FilterKey>

export function SiteFilters({
  onApply,
  value,
}: {
  onApply: (filters: FilterState) => void
  value: FilterState
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<FilterState>(value)
  useEffect(() => {
    setDraft(value)
  }, [value])
  const count = useMemo(
    () => Object.values(value).reduce((sum, items) => sum + items.length, 0),
    [value]
  )

  const renderGroups = (
    groups: readonly (readonly [FilterKey, readonly string[]])[]
  ) =>
    groups.map(([group, statuses]) => (
      <label className='grid w-full gap-1 text-sm sm:w-52' key={group}>
        <span>{t(dynamicI18nKey('site', `site.filters.${group}`))}</span>
        <Select
          onChange={(event) =>
            setDraft((current) => ({
              ...current,
              [group]: event.target.value ? [event.target.value] : [],
            }))
          }
          value={(draft[group] as string[])[0] ?? ''}
        >
          <option value=''>{t('common.all')}</option>
          {statuses.map((status) => (
            <option key={status} value={status}>
              {t(dynamicI18nKey('site', `site.${group}.${status}`))}
            </option>
          ))}
        </Select>
      </label>
    ))

  return (
    <FilterPanel
      advanced={
        <div className='flex flex-wrap items-end gap-2'>
          {renderGroups(advancedGroups)}
        </div>
      }
      description={t('site.filters.description')}
      expandOnLargeScreen
      hasAdvancedActive={count > 0}
      onApply={() => onApply(draft)}
      onReset={() =>
        setDraft({
          auth: [],
          health: [],
          management: [],
          online: [],
          statistics: [],
        })
      }
      title={t('site.filters.title')}
    >
      <div className='flex flex-wrap items-center gap-x-4 gap-y-1'>
        {renderGroups(primaryGroups)}
      </div>
    </FilterPanel>
  )
}
