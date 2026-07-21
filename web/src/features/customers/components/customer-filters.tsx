import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { FilterPanel } from '@/components/data/filter-panel'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import { customerStatuses } from '../constants'
import type { CustomerSearch, CustomerStatus } from '../types'

export function CustomerFilters({
  onApply,
  value,
}: {
  onApply: (status: CustomerStatus[]) => void
  value: CustomerSearch['status']
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<CustomerStatus[]>(value)
  useEffect(() => {
    setDraft(value)
  }, [value])
  return (
    <FilterPanel
      description={t('customer.filters.description')}
      onApply={() => onApply(draft)}
      onReset={() => setDraft([])}
      title={t('customer.filters.title')}
    >
      <label className='grid w-full gap-1 text-sm sm:w-52'>
        <span>{t('customer.statusLabel')}</span>
        <Select
          onChange={(event) => {
            const status = event.target.value as CustomerStatus
            setDraft(status ? [status] : [])
          }}
          value={draft[0] ?? ''}
        >
          <option value=''>{t('common.all')}</option>
          {customerStatuses.map((status) => (
            <option key={status} value={status}>
              {t(dynamicI18nKey('customer', `customer.status.${status}`))}
            </option>
          ))}
        </Select>
      </label>
    </FilterPanel>
  )
}
