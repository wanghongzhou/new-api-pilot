import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { FacetedFilter } from '@/components/data/faceted-filter'
import { FilterPanel } from '@/components/data/filter-panel'
import { Input } from '@/components/ui/input'
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

const filterGroups = [
  ['management', siteManagementStatuses],
  ['online', siteOnlineStatuses],
  ['auth', siteAuthStatuses],
  ['statistics', siteStatisticsStatuses],
  ['health', siteHealthStatuses],
] as const

type FilterState = Pick<SiteSearch, FilterKey | 'filter'>

export function SiteFilters({
  actions,
  onApply,
  value,
}: {
  actions?: ReactNode
  onApply: (filters: FilterState) => void
  value: FilterState
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<FilterState>(value)
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => {
    setDraft(value)
  }, [value])
  useEffect(
    () => () => {
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    },
    []
  )
  const hasActiveFilters =
    draft.filter.trim() !== '' ||
    filterGroups.some(([group]) => draft[group].length > 0)

  const applyImmediately = (next: FilterState) => {
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    setDraft(next)
    onApply({ ...next, filter: next.filter.trim() })
  }

  const updateKeyword = (filter: string) => {
    const next = { ...draft, filter }
    setDraft(next)
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    searchTimerRef.current = setTimeout(
      () => onApply({ ...next, filter: filter.trim() }),
      400
    )
  }

  const renderGroups = (
    groups: readonly (readonly [FilterKey, readonly string[]])[]
  ) =>
    groups.map(([group, statuses]) => {
      const statusLabel = t(dynamicI18nKey('site', `site.filters.${group}`))
      return (
        <FacetedFilter
          clearLabel={t('common.clearFilters')}
          key={group}
          onChange={(nextValue) =>
            applyImmediately({
              ...draft,
              [group]: nextValue ? [nextValue] : [],
            })
          }
          options={statuses.map((status) => ({
            label: t(dynamicI18nKey('site', `site.${group}.${status}`)),
            value: status,
          }))}
          title={t('site.filters.statusTitle', { status: statusLabel })}
          value={(draft[group] as string[])[0] ?? ''}
        />
      )
    })

  return (
    <FilterPanel
      actions={actions}
      description={t('site.filters.description')}
      hasActiveFilters={hasActiveFilters}
      onReset={() => {
        const reset: FilterState = {
          auth: [],
          filter: '',
          health: [],
          management: [],
          online: [],
          statistics: [],
        }
        applyImmediately(reset)
      }}
      title={t('site.filters.title')}
    >
      <div className='flex flex-wrap items-end gap-2'>
        <label className='grid w-full gap-1 text-sm sm:w-64'>
          <span>{t('sites.search')}</span>
          <Input
            onChange={(event) => updateKeyword(event.target.value)}
            placeholder={t('sites.searchPlaceholder')}
            value={draft.filter}
          />
        </label>
        {renderGroups(filterGroups)}
      </div>
    </FilterPanel>
  )
}
