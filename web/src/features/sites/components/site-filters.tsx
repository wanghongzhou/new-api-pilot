import { FilterIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useMemo, useState } from 'react'
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

import {
  siteAuthStatuses,
  siteHealthStatuses,
  siteManagementStatuses,
  siteOnlineStatuses,
  siteStatisticsStatuses,
} from '../constants'
import type { SiteSearch } from '../types'

type FilterKey = 'management' | 'online' | 'auth' | 'statistics' | 'health'

const groups = [
  ['management', siteManagementStatuses],
  ['online', siteOnlineStatuses],
  ['auth', siteAuthStatuses],
  ['statistics', siteStatisticsStatuses],
  ['health', siteHealthStatuses],
] as const

type FilterState = Pick<SiteSearch, FilterKey>

export function SiteFilters({
  onApply,
  value,
}: {
  onApply: (filters: FilterState) => void
  value: FilterState
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<FilterState>(value)
  useEffect(() => {
    if (open) setDraft(value)
  }, [open, value])
  const count = useMemo(
    () => Object.values(value).reduce((sum, items) => sum + items.length, 0),
    [value]
  )

  const toggle = (group: FilterKey, item: string) => {
    setDraft((current) => {
      const values = current[group] as string[]
      const next = values.includes(item)
        ? values.filter((value) => value !== item)
        : [...values, item]
      return { ...current, [group]: next }
    })
  }

  return (
    <Sheet onOpenChange={setOpen} open={open}>
      <Button onClick={() => setOpen(true)} type='button' variant='outline'>
        <HugeiconsIcon icon={FilterIcon} strokeWidth={2} />
        {t('site.filters.title')}
        {count > 0 && <span className='text-xs'>({count})</span>}
      </Button>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t('site.filters.title')}</SheetTitle>
          <SheetDescription>{t('site.filters.description')}</SheetDescription>
        </SheetHeader>
        <div className='grid gap-5 sm:grid-cols-2'>
          {groups.map(([group, statuses]) => (
            <fieldset className='grid gap-1' key={group}>
              <legend className='mb-1 text-sm font-medium'>
                {t(dynamicI18nKey('site', `site.filters.${group}`))}
              </legend>
              {statuses.map((status) => (
                <label
                  className='hover:bg-muted flex min-h-10 items-center gap-2 rounded-md px-2 text-sm'
                  key={status}
                >
                  <input
                    checked={(draft[group] as string[]).includes(status)}
                    className='accent-primary size-4'
                    onChange={() => toggle(group, status)}
                    type='checkbox'
                  />
                  {t(dynamicI18nKey('site', `site.${group}.${status}`))}
                </label>
              ))}
            </fieldset>
          ))}
        </div>
        <div className='border-border mt-2 flex flex-col-reverse gap-2 border-t pt-4 sm:flex-row sm:justify-end'>
          <Button
            onClick={() =>
              setDraft({
                auth: [],
                health: [],
                management: [],
                online: [],
                statistics: [],
              })
            }
            type='button'
            variant='ghost'
          >
            {t('common.clear')}
          </Button>
          <Button
            onClick={() => {
              onApply(draft)
              setOpen(false)
            }}
            type='button'
          >
            {t('common.apply')}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  )
}
