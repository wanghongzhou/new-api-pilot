import {
  ArrowDown01Icon,
  ArrowUp01Icon,
  ArrowUpDownIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { Column } from '@tanstack/react-table'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from './button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from './dropdown-menu'

export function DataTableColumnHeader<TData>({
  column,
  title,
}: {
  column: Column<TData, unknown>
  title: ReactNode
}) {
  const { t } = useTranslation()

  if (!column.getCanSort()) return <div>{title}</div>

  const sorted = column.getIsSorted()
  const icon =
    sorted === 'desc'
      ? ArrowDown01Icon
      : sorted === 'asc'
        ? ArrowUp01Icon
        : ArrowUpDownIcon

  return (
    <div className='flex items-center space-x-2'>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              className='data-popup-open:bg-accent -ms-3 h-8'
              size='sm'
              variant='ghost'
            />
          }
        >
          <span>{title}</span>
          <HugeiconsIcon className='ms-2 size-4' icon={icon} strokeWidth={2} />
        </DropdownMenuTrigger>
        <DropdownMenuContent align='start'>
          <DropdownMenuItem onClick={() => column.toggleSorting(false)}>
            <HugeiconsIcon
              className='text-muted-foreground/70 size-3.5'
              icon={ArrowUp01Icon}
              strokeWidth={2}
            />
            {t('table.sortAscending')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => column.toggleSorting(true)}>
            <HugeiconsIcon
              className='text-muted-foreground/70 size-3.5'
              icon={ArrowDown01Icon}
              strokeWidth={2}
            />
            {t('table.sortDescending')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
