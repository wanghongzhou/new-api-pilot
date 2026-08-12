import { type ReactNode, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { FilterPanel } from '@/components/data/filter-panel'
import { MultiFacetedFilter } from '@/components/data/multi-faceted-filter'
import { Input } from '@/components/ui/input'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { hasFilterChanges } from '@/lib/filter-state'

import { customerStatuses } from '../constants'
import type { CustomerSearch, CustomerStatus } from '../types'

export function CustomerFilters({
  actions,
  onApply,
  value,
}: {
  actions?: ReactNode
  onApply: (filters: Pick<CustomerSearch, 'filter' | 'status'>) => void
  value: Pick<CustomerSearch, 'filter' | 'status'>
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(value)
  useEffect(() => {
    setDraft(value)
  }, [value])
  const reset = { filter: '', status: [] as CustomerStatus[] }
  return (
    <FilterPanel
      actions={actions}
      description={t('customer.filters.description')}
      hasActiveFilters={hasFilterChanges(draft, reset, ['filter', 'status'])}
      onApply={() => onApply({ ...draft, filter: draft.filter.trim() })}
      onReset={() => {
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
        <MultiFacetedFilter
          clearLabel={t('common.clearFilters')}
          maximumSelected={customerStatuses.length}
          onChange={(statuses) =>
            setDraft((current) => ({
              ...current,
              status: statuses as CustomerStatus[],
            }))
          }
          options={customerStatuses.map((status) => ({
            label: t(dynamicI18nKey('customer', `customer.status.${status}`)),
            value: status,
          }))}
          title={t('customer.statusLabel')}
          values={draft.status}
        />
      </div>
    </FilterPanel>
  )
}
