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
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'

import {
  performanceRangeForHours,
  type PerformanceHistorySearch,
} from '../search'

function inputValue(value: number) {
  return fromUnixSeconds(value).format('YYYY-MM-DDTHH:mm')
}

function parseInput(value: string) {
  const parsed = dayjs.tz(value, 'YYYY-MM-DDTHH:mm', BEIJING_TIMEZONE)
  return parsed.isValid() ? parsed.startOf('hour').unix() : undefined
}

function presetLabel(hours: 24 | 168 | 720, t: (key: string) => string) {
  if (hours === 24) return t('performanceHistory.filters.hours24')
  if (hours === 168) return t('performanceHistory.filters.hours168')
  return t('performanceHistory.filters.hours720')
}

export function PerformanceTimeRangePicker({
  onChange,
  search,
}: {
  onChange: (changes: Partial<PerformanceHistorySearch>) => void
  search: PerformanceHistorySearch
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [draftStart, setDraftStart] = useState(() => inputValue(search.start))
  const [draftEnd, setDraftEnd] = useState(() => inputValue(search.end))
  const [rangeError, setRangeError] = useState(false)
  const label = useMemo(() => {
    if (search.end - search.start === search.hours * 3600) {
      return presetLabel(search.hours, t)
    }
    return `${fromUnixSeconds(search.start).format('MM-DD HH:mm')} ~ ${fromUnixSeconds(search.end).format('MM-DD HH:mm')}`
  }, [search.end, search.hours, search.start, t])

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      setDraftStart(inputValue(search.start))
      setDraftEnd(inputValue(search.end))
      setRangeError(false)
    }
    setOpen(nextOpen)
  }

  const applyPreset = (hours: 24 | 168 | 720) => {
    onChange(performanceRangeForHours(hours))
    setOpen(false)
  }

  const applyCustom = () => {
    const start = parseInput(draftStart)
    const end = parseInput(draftEnd)
    if (
      start == null ||
      end == null ||
      start >= end ||
      end - start > 366 * 24 * 3600
    ) {
      setRangeError(true)
      return
    }
    onChange({ end, hours: search.hours, page: 1, start })
    setRangeError(false)
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
        <span>{t('performanceHistory.filters.timeRange')}</span>
        <span className='text-muted-foreground truncate'>{label}</span>
      </PopoverTrigger>
      <PopoverContent
        align='start'
        className='w-[min(560px,calc(100vw-2rem))] p-3'
      >
        <div className='grid gap-3'>
          <fieldset className='grid gap-2'>
            <legend className='text-muted-foreground text-xs'>
              {t('performanceHistory.filters.quickRange')}
            </legend>
            <div className='flex flex-wrap gap-1.5'>
              {([24, 168, 720] as const).map((hours) => (
                <Button
                  aria-pressed={search.hours === hours}
                  className='h-8 flex-1 px-2 text-xs'
                  key={hours}
                  onClick={() => applyPreset(hours)}
                  size='sm'
                  type='button'
                  variant={search.hours === hours ? 'secondary' : 'outline'}
                >
                  {presetLabel(hours, t)}
                </Button>
              ))}
            </div>
          </fieldset>
          <div className='grid gap-2 sm:grid-cols-[1fr_auto_1fr] sm:items-end'>
            <label className='grid gap-1.5 text-sm'>
              <span className='text-muted-foreground text-xs'>
                {t('performanceHistory.filters.start')}
              </span>
              <Input
                className='h-8 tabular-nums'
                onChange={(event) => setDraftStart(event.target.value)}
                type='datetime-local'
                value={draftStart}
              />
            </label>
            <span className='text-muted-foreground hidden pb-2 text-xs sm:block'>
              ~
            </span>
            <label className='grid gap-1.5 text-sm'>
              <span className='text-muted-foreground text-xs'>
                {t('performanceHistory.filters.end')}
              </span>
              <Input
                className='h-8 tabular-nums'
                onChange={(event) => setDraftEnd(event.target.value)}
                type='datetime-local'
                value={draftEnd}
              />
            </label>
          </div>
          {rangeError && (
            <p className='text-destructive text-xs' role='alert'>
              {t('performanceHistory.filters.invalidRange')}
            </p>
          )}
          <div className='flex justify-end'>
            <Button onClick={applyCustom} size='sm' type='button'>
              {t('performanceHistory.filters.applyTime')}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
