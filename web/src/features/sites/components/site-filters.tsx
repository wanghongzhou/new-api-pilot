import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { FilterPanel } from '@/components/data/filter-panel'
import { Input } from '@/components/ui/input'
import { SelectControl as Select } from '@/components/ui/select-control'
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

type FilterState = Pick<SiteSearch, FilterKey | 'filter'>

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
    () =>
      [...primaryGroups, ...advancedGroups].reduce(
        (sum, [key]) => sum + draft[key].length,
        0
      ),
    [draft]
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
      onApply={() => onApply({ ...draft, filter: draft.filter.trim() })}
      onReset={() => {
        const reset: FilterState = {
          auth: [],
          filter: '',
          health: [],
          management: [],
          online: [],
          statistics: [],
        }
        setDraft(reset)
        onApply(reset)
      }}
      title={t('site.filters.title')}
    >
      <div className='flex flex-wrap items-end gap-2'>
        <label className='grid w-full gap-1 text-sm sm:w-64'>
          <span>{t('sites.search')}</span>
          <Input
            onChange={(event) =>
              setDraft((current) => ({
                ...current,
                filter: event.target.value,
              }))
            }
            placeholder={t('sites.searchPlaceholder')}
            value={draft.filter}
          />
        </label>
        {renderGroups(primaryGroups)}
      </div>
    </FilterPanel>
  )
}
