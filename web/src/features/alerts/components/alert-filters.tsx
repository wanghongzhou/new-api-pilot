import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { FilterPanel } from '@/components/data/filter-panel'
import { Checkbox } from '@/components/ui/checkbox'
import { FormField } from '@/components/ui/form-field'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
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
  const [draft, setDraft] = useState<Draft>(() => draftValue(value))
  const [rangeError, setRangeError] = useState(false)
  useEffect(() => {
    setDraft(draftValue(value))
    setRangeError(false)
  }, [value])
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
  return (
    <FilterPanel
      advanced={
        <div className='flex flex-wrap items-end gap-2'>
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
                setDraft((current) => ({ ...current, end: event.target.value }))
              }
              type='datetime-local'
              value={draft.end}
            />
          </FormField>
        </div>
      }
      description={t('alerts.filters.description')}
      hasAdvancedActive={Boolean(value.siteId || value.start || value.end)}
      onApply={() => {
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
      }}
      onReset={() => {
        setDraft({ end: '', level: [], start: '', status: [], targetType: [] })
        setRangeError(false)
      }}
      title={t('alerts.filters.title')}
    >
      <div className='grid w-full gap-3 lg:grid-cols-3'>
        <fieldset className='min-w-0'>
          <legend className='mb-1 text-sm'>{t('alerts.table.status')}</legend>
          <div className='flex flex-wrap gap-1.5'>
            {alertStatuses.map((status) => (
              <label
                className='hover:bg-muted flex min-h-10 items-center gap-2 rounded-md border px-2.5 text-sm'
                key={status}
              >
                <Checkbox
                  checked={draft.status.includes(status)}
                  onCheckedChange={() => toggle('status', status)}
                />
                {alertStatusText(t, status)}
              </label>
            ))}
          </div>
        </fieldset>
        <fieldset className='min-w-0'>
          <legend className='mb-1 text-sm'>{t('alerts.table.level')}</legend>
          <div className='flex flex-wrap gap-1.5'>
            {alertLevels.map((level) => (
              <label
                className='hover:bg-muted flex min-h-10 items-center gap-2 rounded-md border px-2.5 text-sm'
                key={level}
              >
                <Checkbox
                  checked={draft.level.includes(level)}
                  onCheckedChange={() => toggle('level', level)}
                />
                {alertLevelText(t, level)}
              </label>
            ))}
          </div>
        </fieldset>
        <fieldset className='min-w-0'>
          <legend className='mb-1 text-sm'>
            {t('alerts.filters.targetType')}
          </legend>
          <div className='flex flex-wrap gap-1.5'>
            {alertTargetTypes.map((targetType) => (
              <label
                className='hover:bg-muted flex min-h-10 items-center gap-2 rounded-md border px-2.5 text-sm'
                key={targetType}
              >
                <Checkbox
                  checked={draft.targetType.includes(targetType)}
                  onCheckedChange={() => toggle('targetType', targetType)}
                />
                {alertTargetTypeText(t, targetType)}
              </label>
            ))}
          </div>
        </fieldset>
      </div>
    </FilterPanel>
  )
}
