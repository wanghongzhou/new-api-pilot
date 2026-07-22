import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { FilterPanel } from '@/components/data/filter-panel'
import { Input } from '@/components/ui/input'
import { SelectControl as Select } from '@/components/ui/select-control'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import { customerStatuses } from '../constants'
import type { CustomerSearch, CustomerStatus } from '../types'

export function CustomerFilters({
  onApply,
  value,
}: {
  onApply: (filters: Pick<CustomerSearch, 'filter' | 'status'>) => void
  value: Pick<CustomerSearch, 'filter' | 'status'>
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(value)
  useEffect(() => {
    setDraft(value)
  }, [value])
  return (
    <FilterPanel
      description={t('customer.filters.description')}
      onApply={() => onApply({ ...draft, filter: draft.filter.trim() })}
      onReset={() => {
        const reset = { filter: '', status: [] as CustomerStatus[] }
        setDraft(reset)
        onApply(reset)
      }}
      title={t('customer.filters.title')}
    >
      <div className='flex flex-wrap items-end gap-2'>
        <label className='grid w-full gap-1 text-sm sm:w-64'>
          <span>{t('customers.search')}</span>
          <Input
            onChange={(event) =>
              setDraft((current) => ({
                ...current,
                filter: event.target.value,
              }))
            }
            placeholder={t('customers.searchPlaceholder')}
            value={draft.filter}
          />
        </label>
        <label className='grid w-full gap-1 text-sm sm:w-52'>
          <span>{t('customer.statusLabel')}</span>
          <Select
            onChange={(event) => {
              const status = event.target.value as CustomerStatus
              setDraft((current) => ({
                ...current,
                status: status ? [status] : [],
              }))
            }}
            value={draft.status[0] ?? ''}
          >
            <option value=''>{t('common.all')}</option>
            {customerStatuses.map((status) => (
              <option key={status} value={status}>
                {t(dynamicI18nKey('customer', `customer.status.${status}`))}
              </option>
            ))}
          </Select>
        </label>
      </div>
    </FilterPanel>
  )
}
