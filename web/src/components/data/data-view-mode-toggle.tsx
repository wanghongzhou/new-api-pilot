import { GridViewIcon, TableIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/react'

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export type DataViewMode = 'card' | 'table'

export function DataViewModeToggle({
  ariaLabel,
  cardLabel,
  className,
  onChange,
  tableLabel,
  value,
}: {
  ariaLabel: string
  cardLabel: string
  className?: string
  onChange: (mode: DataViewMode) => void
  tableLabel: string
  value: DataViewMode
}) {
  const segments: Array<{
    icon: IconSvgElement
    label: string
    value: DataViewMode
  }> = [
    { icon: GridViewIcon, label: cardLabel, value: 'card' },
    { icon: TableIcon, label: tableLabel, value: 'table' },
  ]

  return (
    <div
      aria-label={ariaLabel}
      className={cn(
        'bg-muted/60 inline-flex h-8 items-center rounded-lg border p-0.5',
        className
      )}
      role='group'
    >
      {segments.map((segment) => {
        const active = segment.value === value
        return (
          <Tooltip key={segment.value}>
            <TooltipTrigger
              render={
                <button
                  aria-label={segment.label}
                  aria-pressed={active}
                  className={cn(
                    'inline-flex h-full w-7 items-center justify-center rounded-md text-xs font-medium transition-all',
                    active
                      ? 'bg-primary text-primary-foreground shadow-sm'
                      : 'text-muted-foreground hover:text-foreground'
                  )}
                  onClick={() => onChange(segment.value)}
                  type='button'
                >
                  <HugeiconsIcon
                    className='size-3.5'
                    icon={segment.icon}
                    strokeWidth={2}
                  />
                </button>
              }
            />
            <TooltipContent className='text-xs' side='bottom'>
              {segment.label}
            </TooltipContent>
          </Tooltip>
        )
      })}
    </div>
  )
}
