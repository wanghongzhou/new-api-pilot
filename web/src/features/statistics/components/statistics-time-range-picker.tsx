import { Calendar03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { SelectControl as Select } from '@/components/ui/select-control'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'

import { defaultStatisticsRange } from '../search'
import type { StatisticsGranularity, StatisticsSearch } from '../types'

type Preset = 'hours24' | 'days7' | 'days30' | 'months12'

function inputType(granularity: StatisticsGranularity) {
  if (granularity === 'hour') return 'datetime-local'
  if (granularity === 'month') return 'month'
  if (granularity === 'year') return 'number'
  return 'date'
}

function inputValue(timestamp: number, granularity: StatisticsGranularity) {
  const value = fromUnixSeconds(timestamp)
  if (granularity === 'hour') return value.format('YYYY-MM-DDTHH:00')
  if (granularity === 'month') return value.format('YYYY-MM')
  if (granularity === 'year') return value.format('YYYY')
  return value.format('YYYY-MM-DD')
}

function parseInput(value: string, granularity: StatisticsGranularity) {
  let format = 'YYYY-MM-DD'
  if (granularity === 'hour') format = 'YYYY-MM-DDTHH:mm'
  else if (granularity === 'month') format = 'YYYY-MM'
  else if (granularity === 'year') format = 'YYYY'
  const parsed = dayjs.tz(value, format, BEIJING_TIMEZONE)
  return parsed.isValid() ? parsed.startOf(granularity).unix() : undefined
}

function presetRange(preset: Preset) {
  const now = dayjs().tz(BEIJING_TIMEZONE)
  if (preset === 'hours24') {
    const end = now.startOf('hour')
    return {
      end: end.unix(),
      granularity: 'hour' as const,
      start: end.subtract(24, 'hour').unix(),
    }
  }
  if (preset === 'months12') {
    const end = now.startOf('month').add(1, 'month')
    return {
      end: end.unix(),
      granularity: 'month' as const,
      start: end.subtract(12, 'month').unix(),
    }
  }
  const end = now.startOf('day').add(1, 'day')
  const days = preset === 'days7' ? 7 : 30
  return {
    end: end.unix(),
    granularity: 'day' as const,
    start: end.subtract(days, 'day').unix(),
  }
}

export function StatisticsTimeRangePicker({
  onChange,
  search,
}: {
  onChange: (changes: Partial<StatisticsSearch>) => void
  search: StatisticsSearch
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [draftGranularity, setDraftGranularity] = useState(search.granularity)
  const [draftStart, setDraftStart] = useState(() =>
    inputValue(search.start, search.granularity)
  )
  const [draftEnd, setDraftEnd] = useState(() =>
    inputValue(search.end, search.granularity)
  )
  const [rangeError, setRangeError] = useState(false)
  const label = useMemo(
    () =>
      `${inputValue(search.start, search.granularity)} ~ ${inputValue(search.end, search.granularity)}`,
    [search.end, search.granularity, search.start]
  )

  const syncDraft = (
    granularity: StatisticsGranularity,
    start: number,
    end: number
  ) => {
    setDraftGranularity(granularity)
    setDraftStart(inputValue(start, granularity))
    setDraftEnd(inputValue(end, granularity))
    setRangeError(false)
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      syncDraft(search.granularity, search.start, search.end)
    }
    setOpen(nextOpen)
  }

  const applyPreset = (preset: Preset) => {
    const range = presetRange(preset)
    syncDraft(range.granularity, range.start, range.end)
  }

  const changeGranularity = (granularity: StatisticsGranularity) => {
    const range = defaultStatisticsRange(granularity)
    syncDraft(granularity, range.start, range.end)
  }

  const apply = () => {
    const start = parseInput(draftStart, draftGranularity)
    const end = parseInput(draftEnd, draftGranularity)
    if (start == null || end == null || start >= end) {
      setRangeError(true)
      return
    }
    onChange({ end, granularity: draftGranularity, page: 1, start })
    setOpen(false)
  }

  return (
    <Popover onOpenChange={handleOpenChange} open={open}>
      <PopoverTrigger
        render={
          <Button
            className='max-w-full justify-start gap-2 px-2.5 font-normal tabular-nums'
            type='button'
            variant='outline'
          />
        }
      >
        <HugeiconsIcon icon={Calendar03Icon} size={16} strokeWidth={2} />
        <span>{t('statistics.timeRange')}</span>
        <span className='text-muted-foreground truncate'>{label}</span>
      </PopoverTrigger>
      <PopoverContent
        align='start'
        className='w-[min(600px,calc(100vw-2rem))] p-3'
      >
        <div className='grid gap-4'>
          <fieldset className='grid gap-2'>
            <legend className='text-muted-foreground text-xs'>
              {t('statistics.quickRange')}
            </legend>
            <div className='grid grid-cols-2 gap-1.5 sm:grid-cols-4'>
              {(['hours24', 'days7', 'days30', 'months12'] as const).map(
                (preset) => (
                  <Button
                    className='h-10 px-2 text-xs sm:h-8'
                    key={preset}
                    onClick={() => applyPreset(preset)}
                    size='sm'
                    type='button'
                    variant='outline'
                  >
                    {t(
                      dynamicI18nKey(
                        'statistics',
                        `statistics.quickRange.${preset}`
                      )
                    )}
                  </Button>
                )
              )}
            </div>
          </fieldset>
          <label className='grid gap-1.5 text-sm'>
            <span className='text-muted-foreground text-xs'>
              {t('statistics.granularity')}
            </span>
            <Select
              className='h-8'
              onChange={(event) =>
                changeGranularity(event.target.value as StatisticsGranularity)
              }
              value={draftGranularity}
            >
              {(['hour', 'day', 'month', 'year'] as const).map(
                (granularity) => (
                  <option key={granularity} value={granularity}>
                    {t(
                      dynamicI18nKey(
                        'statistics',
                        `statistics.granularity.${granularity}`
                      )
                    )}
                  </option>
                )
              )}
            </Select>
          </label>
          <div className='grid gap-2 sm:grid-cols-[1fr_auto_1fr] sm:items-end'>
            <label className='grid gap-1.5 text-sm'>
              <span className='text-muted-foreground text-xs'>
                {t('statistics.start')}
              </span>
              <Input
                className='h-8 tabular-nums'
                onChange={(event) => setDraftStart(event.target.value)}
                type={inputType(draftGranularity)}
                value={draftStart}
              />
            </label>
            <span className='text-muted-foreground hidden pb-2 text-xs sm:block'>
              ~
            </span>
            <label className='grid gap-1.5 text-sm'>
              <span className='text-muted-foreground text-xs'>
                {t('statistics.end')}
              </span>
              <Input
                className='h-8 tabular-nums'
                onChange={(event) => setDraftEnd(event.target.value)}
                type={inputType(draftGranularity)}
                value={draftEnd}
              />
            </label>
          </div>
          {rangeError && (
            <p className='text-destructive text-xs' role='alert'>
              {t('statistics.invalidRange')}
            </p>
          )}
          <div className='flex justify-end'>
            <Button onClick={apply} size='sm' type='button'>
              {t('statistics.applyTimeRange')}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
