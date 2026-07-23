import { Add01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useRef } from 'react'

import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'

export type FacetedFilterOption = {
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
  const selected = options.find((option) => option.value === value)
  const detailsRef = useRef<HTMLDetailsElement>(null)

  useEffect(() => {
    const closeOnOutsidePointer = (event: PointerEvent) => {
      const details = detailsRef.current
      if (details && !details.contains(event.target as Node)) {
        details.removeAttribute('open')
      }
    }
    document.addEventListener('pointerdown', closeOnOutsidePointer)
    return () =>
      document.removeEventListener('pointerdown', closeOnOutsidePointer)
  }, [])

  return (
    <details className='group relative' ref={detailsRef}>
      <summary
        className={cn(
          buttonVariants({ size: 'sm', variant: 'outline' }),
          'h-8 cursor-pointer list-none border-dashed [&::-webkit-details-marker]:hidden'
        )}
      >
        <span className='flex size-4 items-center justify-center rounded-full border'>
          <HugeiconsIcon icon={Add01Icon} size={12} strokeWidth={2} />
        </span>
        {title}
        {selected && (
          <>
            <Separator className='mx-1 h-4' orientation='vertical' />
            <Badge className='rounded-sm px-1 font-normal' variant='secondary'>
              {selected.label}
            </Badge>
          </>
        )}
      </summary>

      <div className='bg-popover text-popover-foreground absolute top-9 left-0 z-30 grid min-w-52 rounded-lg border p-1 shadow-lg'>
        {options.map((option) => {
          const active = option.value === value
          return (
            <button
              className='hover:bg-muted flex min-h-9 items-center gap-2 rounded-md px-2 text-left text-sm'
              key={option.value}
              onClick={(event) => {
                onChange(active ? '' : option.value)
                event.currentTarget.closest('details')?.removeAttribute('open')
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
            </button>
          )
        })}
        {selected && (
          <>
            <Separator className='my-1' />
            <button
              className='hover:bg-muted min-h-9 rounded-md px-2 text-center text-sm'
              onClick={(event) => {
                onChange('')
                event.currentTarget.closest('details')?.removeAttribute('open')
              }}
              type='button'
            >
              {clearLabel}
            </button>
          </>
        )}
      </div>
    </details>
  )
}
