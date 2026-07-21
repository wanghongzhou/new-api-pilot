import { ArrowUpDownIcon } from '@hugeicons/core-free-icons'
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
import { DataTablePagination } from './data-table-pagination'
import { Spinner } from './spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from './table'

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
  onPageSizeChange?: (pageSize: number) => void
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
  onPageSizeChange,
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
          'border-border bg-background overflow-hidden rounded-md border',
          renderMobileCard && 'hidden sm:block'
        )}
      >
        <div
          aria-label={ariaLabel}
          className='focus-visible:ring-ring overflow-x-auto focus-visible:ring-2 focus-visible:outline-none'
          role='region'
          tabIndex={0}
        >
          <Table
            aria-label={ariaLabel}
            className='w-full border-collapse text-sm'
          >
            <TableHeader className='bg-[var(--table-header)] text-left'>
              {table.getHeaderGroups().map((headerGroup) => (
                <TableRow key={headerGroup.id}>
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
                          className='focus-visible:ring-ring inline-flex min-h-8 items-center gap-1 rounded-md outline-none focus-visible:ring-2'
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
                      <TableHead
                        aria-sort={ariaSort}
                        className='text-muted-foreground px-3 py-2 text-[10px] font-medium tracking-wider whitespace-nowrap uppercase'
                        key={header.id}
                      >
                        {content}
                      </TableHead>
                    )
                  })}
                </TableRow>
              ))}
            </TableHeader>
            <TableBody>
              {table.getRowModel().rows.map((row) => (
                <TableRow
                  className='border-t align-top transition-colors hover:bg-[var(--table-header-hover)]'
                  key={row.id}
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell className='px-3 py-2.5' key={cell.id}>
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext()
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
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
        <DataTablePagination
          onPageChange={onPageChange}
          onPageSizeChange={onPageSizeChange}
          page={page}
          pageSize={pageSize}
          total={total}
        />
      )}
    </div>
  )
}
