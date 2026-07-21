import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { FilterPanel } from '@/components/data/filter-panel'
import { NativeSelect as Select } from '@/components/ui/native-select'
import type { CustomerListItem } from '@/features/customers/types'
import type { SiteListItem } from '@/features/sites/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { parseIdString } from '@/lib/api-types'

import {
  accountManagedStatuses,
  accountRemoteStates,
  remoteUserStatusFilters,
} from '../constants'
import type { AccountSearch } from '../types'

type Draft = Pick<
  AccountSearch,
  'customerId' | 'managedStatus' | 'remoteState' | 'remoteStatus' | 'siteId'
>

export function AccountFilters({
  customers,
  onApply,
  sites,
  value,
}: {
  customers: CustomerListItem[]
  onApply: (value: Draft) => void
  sites: SiteListItem[]
  value: Draft
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<Draft>(value)
  useEffect(() => {
    setDraft(value)
  }, [value])
  const select = (
    key: 'managedStatus' | 'remoteState' | 'remoteStatus',
    item: string
  ) => {
    setDraft((current) => ({ ...current, [key]: item ? [item] : [] }))
  }
  return (
    <FilterPanel
      description={t('account.filters.description')}
      onApply={() => onApply(draft)}
      onReset={() =>
        setDraft({
          customerId: undefined,
          managedStatus: [],
          remoteState: [],
          remoteStatus: [],
          siteId: undefined,
        })
      }
      title={t('account.filters.title')}
    >
      <div className='flex flex-wrap items-end gap-2'>
        <label className='grid w-full gap-1 text-sm sm:w-52'>
          <span>{t('account.site')}</span>
          <Select
            onChange={(event) => {
              const value = event.target.value
              setDraft((current) => ({
                ...current,
                siteId: value ? parseIdString(value) : undefined,
              }))
            }}
            value={draft.siteId ?? ''}
          >
            <option value=''>{t('common.all')}</option>
            {sites.map((site) => (
              <option key={site.id} value={site.id}>
                {site.name}
              </option>
            ))}
          </Select>
        </label>
        <label className='grid w-full gap-1 text-sm sm:w-52'>
          <span>{t('account.customer')}</span>
          <Select
            onChange={(event) => {
              const value = event.target.value
              setDraft((current) => ({
                ...current,
                customerId: value ? parseIdString(value) : undefined,
              }))
            }}
            value={draft.customerId ?? ''}
          >
            <option value=''>{t('common.all')}</option>
            {customers.map((customer) => (
              <option key={customer.id} value={customer.id}>
                {customer.name}
              </option>
            ))}
          </Select>
        </label>
        <label className='grid w-full gap-1 text-sm sm:w-52'>
          <span>{t('account.remoteStateLabel')}</span>
          <Select
            onChange={(event) => select('remoteState', event.target.value)}
            value={draft.remoteState[0] ?? ''}
          >
            <option value=''>{t('common.all')}</option>
            {accountRemoteStates.map((state) => (
              <option key={state} value={state}>
                {t(dynamicI18nKey('account', `account.remoteState.${state}`))}
              </option>
            ))}
          </Select>
        </label>
        <label className='grid w-full gap-1 text-sm sm:w-52'>
          <span>{t('account.managedStatusLabel')}</span>
          <Select
            onChange={(event) => select('managedStatus', event.target.value)}
            value={draft.managedStatus[0] ?? ''}
          >
            <option value=''>{t('common.all')}</option>
            {accountManagedStatuses.map((status) => (
              <option key={status} value={status}>
                {t(
                  dynamicI18nKey('account', `account.managedStatus.${status}`)
                )}
              </option>
            ))}
          </Select>
        </label>
        <label className='grid w-full gap-1 text-sm sm:w-52'>
          <span>{t('account.remoteStatusLabel')}</span>
          <Select
            onChange={(event) => select('remoteStatus', event.target.value)}
            value={draft.remoteStatus[0] ?? ''}
          >
            <option value=''>{t('common.all')}</option>
            {remoteUserStatusFilters.map((status) => (
              <option key={status} value={status}>
                {t(
                  dynamicI18nKey(
                    'account',
                    status === '1'
                      ? 'account.remoteStatus.enabled'
                      : 'account.remoteStatus.disabled'
                  )
                )}
              </option>
            ))}
          </Select>
        </label>
      </div>
    </FilterPanel>
  )
}
