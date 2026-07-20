import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  ArrowUpDownIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
  type OnChangeFn,
  type SortingState,
} from '@tanstack/react-table'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { Button } from './button'
import { Spinner } from './spinner'

interface DataTableProps<TData> {
  ariaLabel: string
  columns: ColumnDef<TData, unknown>[]
  data: TData[]
  emptyAction?: ReactNode
  emptyDescription?: string
  emptyTitle?: string
  error?: boolean
  fetching?: boolean
  loading?: boolean
  onPageChange?: (page: number) => void
  onRetry?: () => void
  onSortingChange?: OnChangeFn<SortingState>
  page?: number
  pageSize?: number
  renderMobileCard?: (item: TData) => ReactNode
  sorting?: SortingState
  total?: number
}

export function DataTable<TData>({
  ariaLabel,
  columns,
  data,
  emptyAction,
  emptyDescription,
  emptyTitle,
  error = false,
  fetching = false,
  loading = false,
  onPageChange,
  onRetry,
  onSortingChange,
  page = 1,
  pageSize = 20,
  renderMobileCard,
  sorting = [],
  total = data.length,
}: DataTableProps<TData>) {
  const { t } = useTranslation()
  const table = useReactTable({
    columns,
    data,
    getCoreRowModel: getCoreRowModel(),
    manualSorting: true,
    onSortingChange,
    state: { sorting },
  })
  const pageCount = Math.max(1, Math.ceil(total / pageSize))

  if (loading && data.length === 0) {
    return (
      <div
        aria-hidden='true'
        className='border-border bg-muted/40 h-64 animate-pulse rounded-lg border'
      />
    )
  }

  if (error && data.length === 0) {
    return (
      <section className='border-destructive/30 bg-destructive/5 rounded-lg border p-5'>
        <h2 className='font-medium'>{t('table.loadError')}</h2>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t('table.loadErrorDescription')}
        </p>
        {onRetry && (
          <Button className='mt-3' onClick={onRetry} variant='outline'>
            {t('common.retry')}
          </Button>
        )}
      </section>
    )
  }

  if (data.length === 0) {
    return (
      <section className='border-border bg-card rounded-lg border px-5 py-10 text-center'>
        <h2 className='font-medium'>{emptyTitle ?? t('table.empty')}</h2>
        {emptyDescription && (
          <p className='text-muted-foreground mt-1 text-sm'>
            {emptyDescription}
          </p>
        )}
        {emptyAction && <div className='mt-4'>{emptyAction}</div>}
      </section>
    )
  }

  return (
    <div className='grid min-w-0 gap-3'>
      {fetching && (
        <div
          aria-live='polite'
          className='text-muted-foreground flex min-h-5 items-center gap-2 text-xs'
        >
          <Spinner />
          {t('table.refreshing')}
        </div>
      )}
      <div
        className={cn(
          'overflow-hidden rounded-lg border',
          renderMobileCard && 'hidden sm:block'
        )}
      >
        <div
          aria-label={ariaLabel}
          className='focus-visible:ring-ring overflow-x-auto focus-visible:ring-2 focus-visible:outline-none'
          role='region'
          tabIndex={0}
        >
          <table
            aria-label={ariaLabel}
            className='w-full border-collapse text-sm'
          >
            <thead className='bg-muted/70 text-left'>
              {table.getHeaderGroups().map((headerGroup) => (
                <tr key={headerGroup.id}>
                  {headerGroup.headers.map((header) => {
                    const sorted = header.column.getIsSorted()
                    let ariaSort: 'ascending' | 'descending' | undefined
                    if (sorted === 'asc') ariaSort = 'ascending'
                    else if (sorted === 'desc') ariaSort = 'descending'

                    let content: ReactNode = null
                    if (!header.isPlaceholder) {
                      const label = flexRender(
                        header.column.columnDef.header,
                        header.getContext()
                      )
                      content = header.column.getCanSort() ? (
                        <button
                          className='focus-visible:ring-ring inline-flex min-h-10 items-center gap-1 rounded-sm outline-none focus-visible:ring-2'
                          onClick={header.column.getToggleSortingHandler()}
                          type='button'
                        >
                          {label}
                          <HugeiconsIcon
                            aria-hidden='true'
                            icon={ArrowUpDownIcon}
                            size={14}
                            strokeWidth={2}
                          />
                        </button>
                      ) : (
                        label
                      )
                    }
                    return (
                      <th
                        aria-sort={ariaSort}
                        className='px-3 py-2.5 font-medium whitespace-nowrap'
                        key={header.id}
                      >
                        {content}
                      </th>
                    )
                  })}
                </tr>
              ))}
            </thead>
            <tbody>
              {table.getRowModel().rows.map((row) => (
                <tr className='border-t align-top' key={row.id}>
                  {row.getVisibleCells().map((cell) => (
                    <td className='px-3 py-3' key={cell.id}>
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext()
                      )}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      {renderMobileCard && (
        <div className='grid gap-3 sm:hidden'>
          {data.map((item, index) => (
            <div key={table.getRowModel().rows[index]?.id ?? index}>
              {renderMobileCard(item)}
            </div>
          ))}
        </div>
      )}
      {onPageChange && total > 0 && (
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <p className='text-muted-foreground text-sm'>
            {t('table.total', { total })}
          </p>
          <div className='flex items-center gap-2'>
            <Button
              aria-label={t('table.previous')}
              disabled={page <= 1}
              onClick={() => onPageChange(page - 1)}
              size='icon'
              title={t('table.previous')}
              variant='outline'
            >
              <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            </Button>
            <span className='min-w-24 text-center text-sm'>
              {t('table.page', { page, pages: pageCount })}
            </span>
            <Button
              aria-label={t('table.next')}
              disabled={page >= pageCount}
              onClick={() => onPageChange(page + 1)}
              size='icon'
              title={t('table.next')}
              variant='outline'
            >
              <HugeiconsIcon icon={ArrowRight01Icon} strokeWidth={2} />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
