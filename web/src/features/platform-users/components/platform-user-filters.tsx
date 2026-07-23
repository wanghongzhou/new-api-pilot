import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { FacetedFilter } from '@/components/data/faceted-filter'
import { FilterPanel } from '@/components/data/filter-panel'
import { Input } from '@/components/ui/input'

import type { PlatformUserSearch } from '../types'

type FilterState = Pick<PlatformUserSearch, 'filter' | 'role' | 'status'>

export function PlatformUserFilters({
  onApply,
  value,
}: {
  onApply: (filters: FilterState) => void
  value: FilterState
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(value)
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => setDraft(value), [value])
  useEffect(
    () => () => {
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    },
    []
  )

  const hasActiveFilters =
    draft.filter.trim() !== '' || draft.role != null || draft.status != null

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

  return (
    <FilterPanel
      description={t('Manage platform access and roles')}
      hasActiveFilters={hasActiveFilters}
      onReset={() => {
        const reset: FilterState = {
          filter: '',
          role: undefined,
          status: undefined,
        }
        applyImmediately(reset)
      }}
      title={t('Filter platform users')}
    >
      <div className='flex flex-wrap items-end gap-2'>
        <label className='grid w-full gap-1 text-sm sm:w-64'>
          <span>{t('Search platform users')}</span>
          <Input
            onChange={(event) => updateKeyword(event.target.value)}
            placeholder={t('Search username or display name')}
            value={draft.filter}
          />
        </label>
        <FacetedFilter
          clearLabel={t('common.clearFilters')}
          onChange={(role) =>
            applyImmediately({
              ...draft,
              role: role === '' ? undefined : (role as 'admin' | 'viewer'),
            })
          }
          options={[
            { label: t('Administrator'), value: 'admin' },
            { label: t('Viewer'), value: 'viewer' },
          ]}
          title={t('Role')}
          value={draft.role ?? ''}
        />
        <FacetedFilter
          clearLabel={t('common.clearFilters')}
          onChange={(status) =>
            applyImmediately({
              ...draft,
              status: status === '' ? undefined : (Number(status) as 1 | 2),
            })
          }
          options={[
            { label: t('Enabled'), value: '1' },
            { label: t('Disabled'), value: '2' },
          ]}
          title={t('Status')}
          value={draft.status?.toString() ?? ''}
        />
      </div>
    </FilterPanel>
  )
}
