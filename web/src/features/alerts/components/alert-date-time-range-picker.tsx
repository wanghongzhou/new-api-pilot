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
import { cn } from '@/lib/utils'

function inputValue(timestamp?: number): string {
  return timestamp ? fromUnixSeconds(timestamp).format('YYYY-MM-DDTHH:mm') : ''
}

function timestamp(value: string): number | undefined {
  if (!value) return undefined
  const parsed = dayjs.tz(value, 'YYYY-MM-DDTHH:mm', BEIJING_TIMEZONE)
  return parsed.isValid() ? parsed.unix() : undefined
}

export function AlertDateTimeRangePicker({
  end,
  onChange,
  start,
}: {
  end?: number
  onChange: (range: { end?: number; start?: number }) => void
  start?: number
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [draftStart, setDraftStart] = useState(() => inputValue(start))
  const [draftEnd, setDraftEnd] = useState(() => inputValue(end))
  const [rangeError, setRangeError] = useState(false)
  const label = useMemo(() => {
    if (!start && !end) return t('alerts.filters.dateRange')
    const startText = start
      ? fromUnixSeconds(start).format('YYYY-MM-DD HH:mm')
      : '-'
    const endText = end ? fromUnixSeconds(end).format('YYYY-MM-DD HH:mm') : '-'
    return `${startText} ~ ${endText}`
  }, [end, start, t])

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      setDraftStart(inputValue(start))
      setDraftEnd(inputValue(end))
      setRangeError(false)
    }
    setOpen(nextOpen)
  }

  const apply = (nextStart?: number, nextEnd?: number) => {
    if (nextStart != null && nextEnd != null && nextStart >= nextEnd) {
      setRangeError(true)
      return
    }
    onChange({ end: nextEnd, start: nextStart })
    setRangeError(false)
    setOpen(false)
  }

  const applyPreset = (
    kind: 'today' | 'last7Days' | 'thisWeek' | 'last30Days' | 'thisMonth'
  ) => {
    const now = dayjs().tz(BEIJING_TIMEZONE)
    const ranges = {
      today: {
        end: now.endOf('day'),
        start: now.startOf('day'),
      },
      last7Days: {
        end: now.endOf('day'),
        start: now.subtract(6, 'day').startOf('day'),
      },
      thisWeek: {
        end: now.endOf('week'),
        start: now.startOf('week'),
      },
      last30Days: {
        end: now.endOf('day'),
        start: now.subtract(29, 'day').startOf('day'),
      },
      thisMonth: {
        end: now.endOf('month'),
        start: now.startOf('month'),
      },
    }
    const { end: nextEnd, start: nextStart } = ranges[kind]
    setDraftStart(nextStart.format('YYYY-MM-DDTHH:mm'))
    setDraftEnd(nextEnd.format('YYYY-MM-DDTHH:mm'))
    apply(nextStart.unix(), nextEnd.unix())
  }

  return (
    <Popover onOpenChange={handleOpenChange} open={open}>
      <PopoverTrigger
        render={
          <Button
            className={cn(
              'max-w-full justify-start gap-2 px-2.5 font-normal tabular-nums',
              !start && !end && 'text-muted-foreground'
            )}
            type='button'
            variant='outline'
          />
        }
      >
        <HugeiconsIcon icon={Calendar03Icon} size={16} strokeWidth={2} />
        <span className='truncate'>{label}</span>
      </PopoverTrigger>
      <PopoverContent
        align='start'
        className='w-[min(520px,calc(100vw-2rem))] p-3'
      >
        <div className='grid gap-3'>
          <div className='grid gap-2 sm:grid-cols-[1fr_auto_1fr] sm:items-end'>
            <label className='grid gap-1.5 text-sm'>
              <span className='text-muted-foreground text-xs'>
                {t('alerts.filters.start')}
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
                {t('alerts.filters.end')}
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
              {t('alerts.filters.invalidRange')}
            </p>
          )}
          <div className='flex flex-wrap gap-1.5'>
            {(
              [
                { kind: 'today', label: t('alerts.filters.today') },
                { kind: 'last7Days', label: t('alerts.filters.last7Days') },
                { kind: 'thisWeek', label: t('alerts.filters.thisWeek') },
                {
                  kind: 'last30Days',
                  label: t('alerts.filters.last30Days'),
                },
                { kind: 'thisMonth', label: t('alerts.filters.thisMonth') },
              ] as const
            ).map(({ kind, label }) => (
              <Button
                className='h-7 flex-1 px-2 text-xs'
                key={kind}
                onClick={() => applyPreset(kind)}
                size='sm'
                type='button'
                variant='secondary'
              >
                {label}
              </Button>
            ))}
          </div>
          <div className='flex justify-end'>
            <Button
              className='h-8'
              onClick={() => apply(timestamp(draftStart), timestamp(draftEnd))}
              size='sm'
              type='button'
            >
              {t('alerts.filters.confirmRange')}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
