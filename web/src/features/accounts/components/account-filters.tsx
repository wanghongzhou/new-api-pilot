import { FilterIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Select } from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
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
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<Draft>(value)
  useEffect(() => {
    if (open) setDraft(value)
  }, [open, value])
  const toggle = (
    key: 'managedStatus' | 'remoteState' | 'remoteStatus',
    item: string
  ) => {
    setDraft((current) => {
      const values = current[key] as string[]
      return {
        ...current,
        [key]: values.includes(item)
          ? values.filter((value) => value !== item)
          : [...values, item],
      }
    })
  }
  const count =
    draft.managedStatus.length +
    draft.remoteState.length +
    draft.remoteStatus.length +
    Number(Boolean(draft.siteId)) +
    Number(Boolean(draft.customerId))
  return (
    <Sheet onOpenChange={setOpen} open={open}>
      <Button onClick={() => setOpen(true)} variant='outline'>
        <HugeiconsIcon icon={FilterIcon} strokeWidth={2} />
        {t('account.filters.title')}
        {count > 0 && <span className='text-xs'>({count})</span>}
      </Button>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t('account.filters.title')}</SheetTitle>
          <SheetDescription>
            {t('account.filters.description')}
          </SheetDescription>
        </SheetHeader>
        <div className='grid gap-4 sm:grid-cols-2'>
          <label className='grid gap-1 text-sm'>
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
          <label className='grid gap-1 text-sm'>
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
          <fieldset className='grid gap-1'>
            <legend className='mb-1 text-sm font-medium'>
              {t('account.remoteStateLabel')}
            </legend>
            {accountRemoteStates.map((state) => (
              <label
                className='flex min-h-10 items-center gap-2 text-sm'
                key={state}
              >
                <input
                  checked={draft.remoteState.includes(state)}
                  onChange={() => toggle('remoteState', state)}
                  type='checkbox'
                />
                {t(dynamicI18nKey('account', `account.remoteState.${state}`))}
              </label>
            ))}
          </fieldset>
          <fieldset className='grid gap-1'>
            <legend className='mb-1 text-sm font-medium'>
              {t('account.managedStatusLabel')}
            </legend>
            {accountManagedStatuses.map((status) => (
              <label
                className='flex min-h-10 items-center gap-2 text-sm'
                key={status}
              >
                <input
                  checked={draft.managedStatus.includes(status)}
                  onChange={() => toggle('managedStatus', status)}
                  type='checkbox'
                />
                {t(
                  dynamicI18nKey('account', `account.managedStatus.${status}`)
                )}
              </label>
            ))}
          </fieldset>
          <fieldset className='grid gap-1'>
            <legend className='mb-1 text-sm font-medium'>
              {t('account.remoteStatusLabel')}
            </legend>
            {remoteUserStatusFilters.map((status) => (
              <label
                className='flex min-h-10 items-center gap-2 text-sm'
                key={status}
              >
                <input
                  checked={draft.remoteStatus.includes(status)}
                  onChange={() => toggle('remoteStatus', status)}
                  type='checkbox'
                />
                {t(
                  dynamicI18nKey(
                    'account',
                    status === '1'
                      ? 'account.remoteStatus.enabled'
                      : 'account.remoteStatus.disabled'
                  )
                )}
              </label>
            ))}
          </fieldset>
        </div>
        <div className='border-border mt-4 flex gap-2 border-t pt-4'>
          <Button
            onClick={() =>
              setDraft({
                customerId: undefined,
                managedStatus: [],
                remoteState: [],
                remoteStatus: [],
                siteId: undefined,
              })
            }
            variant='outline'
          >
            {t('common.reset')}
          </Button>
          <Button
            onClick={() => {
              onApply(draft)
              setOpen(false)
            }}
          >
            {t('common.apply')}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  )
}
