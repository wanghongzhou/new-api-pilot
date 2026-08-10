import {
  ArrowLeft01Icon,
  ArrowLeftDoubleIcon,
  ArrowRight01Icon,
  ArrowRightDoubleIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { getPageNumbers } from '@/lib/utils'

import { Button } from './button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './select'

const PAGE_SIZE_OPTIONS = [10, 20, 30, 40, 50, 100] as const
const PAGE_SIZE_SELECT_ITEMS = PAGE_SIZE_OPTIONS.map((pageSize) => ({
  label: pageSize,
  value: `${pageSize}`,
}))

interface DataTablePaginationProps {
  hasKnownLastPage?: boolean
  hasNextPage?: boolean
  onPageChange: (page: number) => void
  onPageSizeChange?: (pageSize: number) => void
  page: number
  pageSize: number
  total: number
  totalDisplay?: ReactNode
}

export function DataTablePagination({
  hasKnownLastPage = true,
  hasNextPage,
  onPageChange,
  onPageSizeChange,
  page,
  pageSize,
  total,
  totalDisplay,
}: DataTablePaginationProps) {
  const { t } = useTranslation()
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const currentPage = Math.min(Math.max(page, 1), totalPages)
  const pageNumbers = getPageNumbers(currentPage, totalPages)
  const canGoPrevious = currentPage > 1
  const canGoNext = hasNextPage ?? currentPage < totalPages

  return (
    <div
      className='@container/pagination flex min-w-0 items-center justify-end overflow-clip'
      style={{ overflowClipMargin: 1 }}
    >
      <div className='flex min-w-0 shrink-0 items-center gap-2 @xl/pagination:gap-3'>
        <div className='flex shrink-0 items-baseline gap-1.5 text-xs font-medium whitespace-nowrap sm:text-sm'>
          <span className='text-muted-foreground'>{t('table.totalLabel')}</span>
          <span className='text-foreground tabular-nums'>
            {totalDisplay ?? total.toLocaleString()}
          </span>
        </div>

        {onPageSizeChange && (
          <div className='flex shrink-0 items-center gap-1.5 @lg/pagination:gap-2'>
            <p className='text-muted-foreground hidden text-sm font-medium whitespace-nowrap @2xl/pagination:block'>
              {t('table.rowsPerPage')}
            </p>
            <Select
              items={PAGE_SIZE_SELECT_ITEMS}
              onValueChange={(value) => {
                const nextPageSize = Number(value)
                if (
                  Number.isSafeInteger(nextPageSize) &&
                  nextPageSize > 0 &&
                  nextPageSize !== pageSize
                ) {
                  onPageSizeChange(nextPageSize)
                }
              }}
              value={`${pageSize}`}
            >
              <SelectTrigger
                aria-label={t('table.rowsPerPage')}
                className='text-foreground w-[64px] font-medium tabular-nums data-[size=default]:h-10 sm:w-[70px] sm:data-[size=default]:h-8'
              >
                <SelectValue placeholder={pageSize} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false} side='top'>
                <SelectGroup>
                  {PAGE_SIZE_OPTIONS.map((option) => (
                    <SelectItem key={option} value={`${option}`}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
        )}

        <div className='flex min-w-0 shrink-0 items-center gap-1 @lg/pagination:gap-1.5 @xl/pagination:gap-2'>
          <Button
            className='text-muted-foreground hover:text-foreground disabled:text-muted-foreground/50 size-10 p-0 sm:size-8 @max-lg/pagination:hidden'
            disabled={!canGoPrevious}
            onClick={() => onPageChange(1)}
            variant='outline'
          >
            <span className='sr-only'>{t('table.first')}</span>
            <HugeiconsIcon
              icon={ArrowLeftDoubleIcon}
              size={16}
              strokeWidth={2}
            />
          </Button>
          <Button
            className='text-muted-foreground hover:text-foreground disabled:text-muted-foreground/50 size-10 p-0 sm:size-8'
            disabled={!canGoPrevious}
            onClick={() => onPageChange(currentPage - 1)}
            variant='outline'
          >
            <span className='sr-only'>{t('table.previous')}</span>
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} strokeWidth={2} />
          </Button>

          <div className='hidden items-center gap-1 @lg/pagination:flex @xl/pagination:gap-2'>
            {pageNumbers.map((pageNumber, index) => (
              <div className='flex items-center' key={`${pageNumber}-${index}`}>
                {pageNumber === '...' ? (
                  <span className='text-muted-foreground px-0.5 text-sm @lg/pagination:px-1'>
                    ...
                  </span>
                ) : (
                  <Button
                    className={`h-8 min-w-8 px-2 tabular-nums ${
                      currentPage === pageNumber
                        ? 'font-semibold'
                        : 'text-muted-foreground hover:text-foreground'
                    }`}
                    onClick={() => onPageChange(pageNumber as number)}
                    variant={currentPage === pageNumber ? 'default' : 'outline'}
                  >
                    <span className='sr-only'>
                      {t('table.goToPage', { page: pageNumber })}
                    </span>
                    {pageNumber}
                  </Button>
                )}
              </div>
            ))}
          </div>

          <Button
            className='text-muted-foreground hover:text-foreground disabled:text-muted-foreground/50 size-10 p-0 sm:size-8'
            disabled={!canGoNext}
            onClick={() => onPageChange(currentPage + 1)}
            variant='outline'
          >
            <span className='sr-only'>{t('table.next')}</span>
            <HugeiconsIcon icon={ArrowRight01Icon} size={16} strokeWidth={2} />
          </Button>
          {hasKnownLastPage && (
            <Button
              className='text-muted-foreground hover:text-foreground disabled:text-muted-foreground/50 size-10 p-0 sm:size-8 @max-lg/pagination:hidden'
              disabled={!canGoNext}
              onClick={() => onPageChange(totalPages)}
              variant='outline'
            >
              <span className='sr-only'>{t('table.last')}</span>
              <HugeiconsIcon
                icon={ArrowRightDoubleIcon}
                size={16}
                strokeWidth={2}
              />
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
