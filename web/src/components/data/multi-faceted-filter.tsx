import { Add01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'

import type { FacetedFilterOption } from './faceted-filter'

export function MultiFacetedFilter({
  className,
  clearLabel,
  maximumSelected = 100,
  onChange,
  options,
  title,
  values,
}: {
  className?: string
  clearLabel: string
  maximumSelected?: number
  onChange: (values: string[]) => void
  options: FacetedFilterOption[]
  title: string
  values: readonly string[]
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const selected = useMemo(() => new Set(values), [values])
  const visibleOptions = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase()
    if (!normalized) return options
    return options.filter((option) =>
      option.label.toLocaleLowerCase().includes(normalized)
    )
  }, [options, query])
  const renderedOptions = visibleOptions.slice(0, 200)

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger
        render={
          <Button
            className={cn('h-10 border-dashed sm:h-8', className)}
            size='sm'
            variant='outline'
          />
        }
      >
        <HugeiconsIcon icon={Add01Icon} size={14} strokeWidth={2} />
        {title}
        {selected.size > 0 && (
          <>
            <Separator className='mx-1 h-4' orientation='vertical' />
            <Badge className='rounded-sm px-1 font-normal' variant='secondary'>
              {selected.size}
            </Badge>
          </>
        )}
      </PopoverTrigger>
      <PopoverContent
        align='start'
        className='max-w-[360px] min-w-[240px] gap-1 p-1'
      >
        <Input
          aria-label={title}
          className='border-input/30 bg-input/30 h-10 shadow-none sm:h-8'
          onChange={(event) => setQuery(event.target.value)}
          placeholder={title}
          value={query}
        />
        <div className='max-h-72 overflow-y-auto p-1'>
          {renderedOptions.map((option) => {
            const active = selected.has(option.value)
            const selectionLimitReached =
              !active && selected.size >= maximumSelected
            return (
              <button
                aria-pressed={active}
                className='data-[active=true]:bg-muted flex min-h-10 w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-hidden sm:min-h-8'
                data-active={active}
                disabled={selectionLimitReached}
                key={option.value}
                onClick={() =>
                  onChange(
                    active
                      ? values.filter((value) => value !== option.value)
                      : [...values, option.value]
                  )
                }
                type='button'
              >
                <span
                  className={cn(
                    'border-primary flex size-4 items-center justify-center rounded-sm border',
                    active
                      ? 'bg-primary text-primary-foreground'
                      : 'opacity-50 [&_svg]:invisible'
                  )}
                >
                  <HugeiconsIcon icon={Tick02Icon} size={14} strokeWidth={2} />
                </span>
                <span className='min-w-0 flex-1 truncate' title={option.label}>
                  {option.label}
                </span>
              </button>
            )
          })}
          {visibleOptions.length === 0 && (
            <p className='text-muted-foreground py-6 text-center text-sm'>-</p>
          )}
          {visibleOptions.length > renderedOptions.length && (
            <p
              className='text-muted-foreground px-2 py-1.5 text-xs'
              role='status'
            >
              {t('common.moreFilterOptions', {
                count: visibleOptions.length - renderedOptions.length,
              })}
            </p>
          )}
          {selected.size >= maximumSelected && (
            <p
              className='text-muted-foreground px-2 py-1.5 text-xs'
              role='status'
            >
              {t('common.selectionLimit', { count: maximumSelected })}
            </p>
          )}
          {selected.size > 0 && (
            <>
              <Separator className='my-1' />
              <button
                className='hover:bg-muted min-h-10 w-full rounded-sm px-2 py-1.5 text-center text-sm sm:min-h-8'
                onClick={() => {
                  onChange([])
                  setOpen(false)
                  setQuery('')
                }}
                type='button'
              >
                {clearLabel}
              </button>
            </>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
