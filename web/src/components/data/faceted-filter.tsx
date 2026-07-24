import { Add01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMemo, useState } from 'react'

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

export type FacetedFilterOption = {
  count?: number
  label: string
  value: string
}

export function FacetedFilter({
  clearLabel,
  onChange,
  options,
  title,
  value,
}: {
  clearLabel: string
  onChange: (value: string) => void
  options: FacetedFilterOption[]
  title: string
  value: string
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const selected = options.find((option) => option.value === value)
  const visibleOptions = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase()
    if (!normalized) return options
    return options.filter((option) =>
      option.label.toLocaleLowerCase().includes(normalized)
    )
  }, [options, query])

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger
        render={
          <Button variant='outline' size='sm' className='h-8 border-dashed' />
        }
      >
        <HugeiconsIcon icon={Add01Icon} size={14} strokeWidth={2} />
        {title}
        {selected && (
          <>
            <Separator className='mx-1 h-4' orientation='vertical' />
            <Badge className='rounded-sm px-1 font-normal' variant='secondary'>
              {selected.label}
            </Badge>
          </>
        )}
      </PopoverTrigger>
      <PopoverContent
        align='start'
        className='max-w-[360px] min-w-[200px] gap-1 p-1'
      >
        <Input
          aria-label={title}
          className='border-input/30 bg-input/30 h-8 shadow-none'
          onChange={(event) => setQuery(event.target.value)}
          placeholder={title}
          value={query}
        />
        <div className='max-h-72 overflow-y-auto p-1'>
          {visibleOptions.map((option) => {
            const active = option.value === value
            return (
              <button
                className='data-[active=true]:bg-muted flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-hidden'
                data-active={active}
                key={option.value}
                onClick={() => {
                  onChange(active ? '' : option.value)
                  setOpen(false)
                  setQuery('')
                }}
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
                {typeof option.count === 'number' && (
                  <span className='text-muted-foreground font-mono text-xs'>
                    {option.count}
                  </span>
                )}
              </button>
            )
          })}
          {visibleOptions.length === 0 && (
            <p className='text-muted-foreground py-6 text-center text-sm'>-</p>
          )}
          {selected && (
            <>
              <Separator className='my-1' />
              <button
                className='hover:bg-muted w-full rounded-sm px-2 py-1.5 text-center text-sm'
                onClick={() => {
                  onChange('')
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
