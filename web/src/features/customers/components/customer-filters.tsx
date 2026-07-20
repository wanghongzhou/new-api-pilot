import { FilterIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
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
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<CustomerStatus[]>(value)
  useEffect(() => {
    if (open) setDraft(value)
  }, [open, value])
  const toggle = (status: CustomerStatus) => {
    setDraft((current) =>
      current.includes(status)
        ? current.filter((item) => item !== status)
        : [...current, status]
    )
  }
  return (
    <Sheet onOpenChange={setOpen} open={open}>
      <Button onClick={() => setOpen(true)} variant='outline'>
        <HugeiconsIcon icon={FilterIcon} strokeWidth={2} />
        {t('customer.filters.title')}
        {value.length > 0 && <span className='text-xs'>({value.length})</span>}
      </Button>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t('customer.filters.title')}</SheetTitle>
          <SheetDescription>
            {t('customer.filters.description')}
          </SheetDescription>
        </SheetHeader>
        <fieldset className='grid gap-1'>
          <legend className='mb-2 text-sm font-medium'>
            {t('customer.statusLabel')}
          </legend>
          {customerStatuses.map((status) => (
            <label
              className='hover:bg-muted flex min-h-10 items-center gap-2 rounded-md px-2 text-sm'
              key={status}
            >
              <input
                checked={draft.includes(status)}
                className='accent-primary size-4'
                onChange={() => toggle(status)}
                type='checkbox'
              />
              {t(dynamicI18nKey('customer', `customer.status.${status}`))}
            </label>
          ))}
        </fieldset>
        <div className='border-border mt-4 flex gap-2 border-t pt-4'>
          <Button onClick={() => setDraft([])} variant='outline'>
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
