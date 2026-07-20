import { FilterIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { FormField } from '@/components/ui/form-field'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import type { SiteListItem } from '@/features/sites/types'
import { isIdString } from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'

import { alertLevels, alertStatuses, alertTargetTypes } from '../constants'
import type { AlertSearch } from '../types'
import {
  alertLevelText,
  alertStatusText,
  alertTargetTypeText,
} from './alert-ui'

type AlertFilterValue = Pick<
  AlertSearch,
  'end' | 'level' | 'siteId' | 'start' | 'status' | 'targetType'
>

type Draft = Omit<AlertFilterValue, 'end' | 'start'> & {
  end: string
  start: string
}

function inputTime(timestamp?: number): string {
  return timestamp ? fromUnixSeconds(timestamp).format('YYYY-MM-DDTHH:mm') : ''
}

function draftValue(value: AlertFilterValue): Draft {
  return {
    end: inputTime(value.end),
    level: [...value.level],
    siteId: value.siteId,
    start: inputTime(value.start),
    status: [...value.status],
    targetType: [...value.targetType],
  }
}

function timestamp(value: string): number | undefined {
  if (!value) return undefined
  const parsed = dayjs.tz(value, 'YYYY-MM-DDTHH:mm', BEIJING_TIMEZONE)
  return parsed.isValid() ? parsed.unix() : undefined
}

export function AlertFilters({
  onApply,
  sites,
  value,
}: {
  onApply: (value: AlertFilterValue) => void
  sites: SiteListItem[]
  value: AlertFilterValue
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<Draft>(() => draftValue(value))
  const [rangeError, setRangeError] = useState(false)
  useEffect(() => {
    if (open) {
      setDraft(draftValue(value))
      setRangeError(false)
    }
  }, [open, value])
  const toggle = (key: 'level' | 'status' | 'targetType', item: string) => {
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
    value.status.length +
    value.level.length +
    value.targetType.length +
    Number(Boolean(value.siteId)) +
    Number(Boolean(value.start)) +
    Number(Boolean(value.end))
  return (
    <Sheet onOpenChange={setOpen} open={open}>
      <Button onClick={() => setOpen(true)} variant='outline'>
        <HugeiconsIcon icon={FilterIcon} strokeWidth={2} />
        {t('alerts.filters.title')}
        {count > 0 && <span className='text-xs'>({count})</span>}
      </Button>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t('alerts.filters.title')}</SheetTitle>
          <SheetDescription>{t('alerts.filters.description')}</SheetDescription>
        </SheetHeader>
        <div className='grid gap-4 sm:grid-cols-2'>
          <fieldset className='grid gap-1'>
            <legend className='mb-1 text-sm font-medium'>
              {t('alerts.table.status')}
            </legend>
            {alertStatuses.map((status) => (
              <label
                className='hover:bg-muted flex min-h-10 items-center gap-2 rounded-md px-2 text-sm'
                key={status}
              >
                <input
                  checked={draft.status.includes(status)}
                  className='accent-primary size-4'
                  onChange={() => toggle('status', status)}
                  type='checkbox'
                />
                {alertStatusText(t, status)}
              </label>
            ))}
          </fieldset>
          <fieldset className='grid gap-1'>
            <legend className='mb-1 text-sm font-medium'>
              {t('alerts.table.level')}
            </legend>
            {alertLevels.map((level) => (
              <label
                className='hover:bg-muted flex min-h-10 items-center gap-2 rounded-md px-2 text-sm'
                key={level}
              >
                <input
                  checked={draft.level.includes(level)}
                  className='accent-primary size-4'
                  onChange={() => toggle('level', level)}
                  type='checkbox'
                />
                {alertLevelText(t, level)}
              </label>
            ))}
          </fieldset>
          <fieldset className='grid gap-1'>
            <legend className='mb-1 text-sm font-medium'>
              {t('alerts.filters.targetType')}
            </legend>
            {alertTargetTypes.map((targetType) => (
              <label
                className='hover:bg-muted flex min-h-10 items-center gap-2 rounded-md px-2 text-sm'
                key={targetType}
              >
                <input
                  checked={draft.targetType.includes(targetType)}
                  className='accent-primary size-4'
                  onChange={() => toggle('targetType', targetType)}
                  type='checkbox'
                />
                {alertTargetTypeText(t, targetType)}
              </label>
            ))}
          </fieldset>
          <FormField
            htmlFor='alerts-filter-site'
            label={t('alerts.table.site')}
          >
            <Select
              className='w-full min-w-0'
              id='alerts-filter-site'
              onChange={(event) => {
                const siteId = event.target.value
                setDraft((current) => ({
                  ...current,
                  siteId: isIdString(siteId) ? siteId : undefined,
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
          </FormField>
          <FormField
            htmlFor='alerts-filter-start'
            label={t('alerts.filters.start')}
          >
            <Input
              className='min-w-0'
              id='alerts-filter-start'
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  start: event.target.value,
                }))
              }
              type='datetime-local'
              value={draft.start}
            />
          </FormField>
          <FormField
            error={rangeError ? t('alerts.filters.invalidRange') : undefined}
            htmlFor='alerts-filter-end'
            label={t('alerts.filters.end')}
          >
            <Input
              className='min-w-0'
              id='alerts-filter-end'
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  end: event.target.value,
                }))
              }
              type='datetime-local'
              value={draft.end}
            />
          </FormField>
        </div>
        <div className='border-border mt-4 flex flex-wrap gap-2 border-t pt-4'>
          <Button
            onClick={() => {
              setDraft({
                end: '',
                level: [],
                start: '',
                status: [],
                targetType: [],
              })
              setRangeError(false)
            }}
            variant='outline'
          >
            {t('common.reset')}
          </Button>
          <Button
            onClick={() => {
              const start = timestamp(draft.start)
              const end = timestamp(draft.end)
              if (start != null && end != null && start >= end) {
                setRangeError(true)
                return
              }
              onApply({
                end,
                level: draft.level,
                siteId: draft.siteId,
                start,
                status: draft.status,
                targetType: draft.targetType,
              })
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
