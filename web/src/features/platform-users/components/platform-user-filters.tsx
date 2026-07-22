import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { FilterPanel } from '@/components/data/filter-panel'
import { Input } from '@/components/ui/input'
import { SelectControl as Select } from '@/components/ui/select-control'

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

  useEffect(() => setDraft(value), [value])

  return (
    <FilterPanel
      description={t('Manage platform access and roles')}
      onApply={() => onApply({ ...draft, filter: draft.filter.trim() })}
      onReset={() => {
        const reset: FilterState = {
          filter: '',
          role: undefined,
          status: undefined,
        }
        setDraft(reset)
        onApply(reset)
      }}
      title={t('Filter platform users')}
    >
      <div className='flex flex-wrap items-end gap-2'>
        <label className='grid w-full gap-1 text-sm sm:w-64'>
          <span>{t('Search platform users')}</span>
          <Input
            onChange={(event) =>
              setDraft((current) => ({
                ...current,
                filter: event.target.value,
              }))
            }
            placeholder={t('Search username or display name')}
            value={draft.filter}
          />
        </label>
        <label className='grid w-full gap-1 text-sm sm:w-52'>
          <span>{t('Filter by role')}</span>
          <Select
            onChange={(event) =>
              setDraft((current) => ({
                ...current,
                role:
                  event.target.value === ''
                    ? undefined
                    : (event.target.value as 'admin' | 'viewer'),
              }))
            }
            value={draft.role ?? ''}
          >
            <option value=''>{t('All roles')}</option>
            <option value='admin'>{t('Administrator')}</option>
            <option value='viewer'>{t('Viewer')}</option>
          </Select>
        </label>
        <label className='grid w-full gap-1 text-sm sm:w-52'>
          <span>{t('Filter by status')}</span>
          <Select
            onChange={(event) =>
              setDraft((current) => ({
                ...current,
                status:
                  event.target.value === ''
                    ? undefined
                    : (Number(event.target.value) as 1 | 2),
              }))
            }
            value={draft.status?.toString() ?? ''}
          >
            <option value=''>{t('All statuses')}</option>
            <option value='1'>{t('Enabled')}</option>
            <option value='2'>{t('Disabled')}</option>
          </Select>
        </label>
      </div>
    </FilterPanel>
  )
}
