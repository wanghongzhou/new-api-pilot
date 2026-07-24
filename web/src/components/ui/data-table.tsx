import { Alert02Icon, Database01Icon } from '@hugeicons/core-free-icons'
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

import { PageFooterPortal } from '../layout/page-footer'
import { Button } from './button'
import { DataTableColumnHeader } from './data-table-column-header'
import { DataTablePagination } from './data-table-pagination'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from './empty'
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
  fillAvailableHeight?: boolean
  loading?: boolean
  onPageChange?: (page: number) => void
  onPageSizeChange?: (pageSize: number) => void
  onRetry?: () => void
  onSortingChange?: OnChangeFn<SortingState>
  page?: number
  pageSize?: number
  paginationInFooter?: boolean
  preserveHeaderWhenEmpty?: boolean
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
  fillAvailableHeight = true,
  loading = false,
  onPageChange,
  onPageSizeChange,
  onRetry,
  onSortingChange,
  page = 1,
  pageSize = 20,
  paginationInFooter = true,
  preserveHeaderWhenEmpty = true,
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
  const paginationControl = onPageChange ? (
    <DataTablePagination
      onPageChange={onPageChange}
      onPageSizeChange={onPageSizeChange}
      page={page}
      pageSize={pageSize}
      total={total}
    />
  ) : null
  const pagination =
    paginationControl && paginationInFooter ? (
      <PageFooterPortal>{paginationControl}</PageFooterPortal>
    ) : (
      paginationControl
    )

  if (loading && data.length === 0 && !preserveHeaderWhenEmpty) {
    return (
      <div className='grid min-w-0 gap-3'>
        <div
          aria-hidden='true'
          className='border-border bg-muted/40 h-64 animate-pulse rounded-lg border'
        />
        {pagination}
      </div>
    )
  }

  if (error && data.length === 0 && !preserveHeaderWhenEmpty) {
    return (
      <div className='grid min-w-0 gap-3'>
        <Empty className='border-border bg-background min-h-64 border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon
                className='text-destructive size-6'
                icon={Alert02Icon}
                strokeWidth={2}
              />
            </EmptyMedia>
            <EmptyTitle>{t('table.loadError')}</EmptyTitle>
            <EmptyDescription>
              {t('table.loadErrorDescription')}
            </EmptyDescription>
          </EmptyHeader>
          {onRetry && (
            <EmptyContent>
              <Button onClick={onRetry} size='sm' variant='outline'>
                {t('common.retry')}
              </Button>
            </EmptyContent>
          )}
        </Empty>
        {pagination}
      </div>
    )
  }

  if (data.length === 0 && !preserveHeaderWhenEmpty) {
    return (
      <div className='grid min-w-0 gap-3'>
        <Empty className='border-border bg-background min-h-64 border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon
                className='size-6'
                icon={Database01Icon}
                strokeWidth={2}
              />
            </EmptyMedia>
            <EmptyTitle>{emptyTitle ?? t('table.empty')}</EmptyTitle>
            {emptyDescription && (
              <EmptyDescription>{emptyDescription}</EmptyDescription>
            )}
          </EmptyHeader>
          {emptyAction && <EmptyContent>{emptyAction}</EmptyContent>}
        </Empty>
        {pagination}
      </div>
    )
  }

  let emptyTableBody: ReactNode = null
  if (preserveHeaderWhenEmpty && data.length === 0) {
    let content: ReactNode
    if (loading) {
      content = (
        <div aria-hidden='true' className='bg-muted/40 h-56 animate-pulse' />
      )
    } else if (error) {
      content = (
        <Empty className='min-h-56 rounded-none border-0'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon
                className='text-destructive size-6'
                icon={Alert02Icon}
                strokeWidth={2}
              />
            </EmptyMedia>
            <EmptyTitle>{t('table.loadError')}</EmptyTitle>
            <EmptyDescription>
              {t('table.loadErrorDescription')}
            </EmptyDescription>
          </EmptyHeader>
          {onRetry && (
            <EmptyContent>
              <Button onClick={onRetry} size='sm' variant='outline'>
                {t('common.retry')}
              </Button>
            </EmptyContent>
          )}
        </Empty>
      )
    } else {
      content = (
        <Empty className='min-h-56 rounded-none border-0'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon
                className='size-6'
                icon={Database01Icon}
                strokeWidth={2}
              />
            </EmptyMedia>
            <EmptyTitle>{emptyTitle ?? t('table.empty')}</EmptyTitle>
            {emptyDescription && (
              <EmptyDescription>{emptyDescription}</EmptyDescription>
            )}
          </EmptyHeader>
          {emptyAction && <EmptyContent>{emptyAction}</EmptyContent>}
        </Empty>
      )
    }
    emptyTableBody = (
      <TableRow>
        <TableCell
          className='h-[400px] p-0'
          colSpan={Math.max(1, table.getVisibleLeafColumns().length)}
        >
          {content}
        </TableCell>
      </TableRow>
    )
  }

  return (
    <div
      className={
        fillAvailableHeight
          ? 'flex min-h-0 min-w-0 flex-1 flex-col gap-2.5 sm:gap-3'
          : 'grid min-w-0 gap-3'
      }
    >
      <div
        className={cn(
          'border-border bg-background overflow-hidden rounded-md border transition-opacity duration-150',
          fillAvailableHeight && 'min-h-0 flex-1',
          fetching && !loading && 'pointer-events-none opacity-60',
          renderMobileCard && 'hidden min-[641px]:block'
        )}
      >
        <div
          aria-label={ariaLabel}
          aria-busy={loading || fetching}
          className={cn(
            'focus-visible:ring-ring overflow-x-auto focus-visible:ring-2 focus-visible:outline-none',
            fillAvailableHeight && 'h-full'
          )}
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
                      content = (
                        <DataTableColumnHeader
                          column={header.column}
                          title={label}
                        />
                      )
                    }
                    return (
                      <TableHead
                        aria-sort={ariaSort}
                        className='relative px-3'
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
              {emptyTableBody ??
                table.getRowModel().rows.map((row) => (
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
        <div
          aria-busy={loading || fetching}
          className={cn(
            'grid min-h-0 flex-1 gap-3 overflow-y-auto transition-opacity duration-150 min-[641px]:hidden',
            fetching && !loading && 'pointer-events-none opacity-60'
          )}
        >
          {loading && data.length === 0 ? (
            Array.from({ length: 3 }, (_, index) => (
              <div
                aria-hidden='true'
                className='bg-muted/40 h-40 animate-pulse rounded-xl border'
                key={index}
              />
            ))
          ) : error && data.length === 0 ? (
            <Empty className='min-h-[300px]'>
              <EmptyHeader>
                <EmptyMedia variant='icon'>
                  <HugeiconsIcon
                    className='text-destructive size-6'
                    icon={Alert02Icon}
                    strokeWidth={2}
                  />
                </EmptyMedia>
                <EmptyTitle>{t('table.loadError')}</EmptyTitle>
                <EmptyDescription>
                  {t('table.loadErrorDescription')}
                </EmptyDescription>
              </EmptyHeader>
              {onRetry && (
                <EmptyContent>
                  <Button onClick={onRetry} size='sm' variant='outline'>
                    {t('common.retry')}
                  </Button>
                </EmptyContent>
              )}
            </Empty>
          ) : data.length === 0 ? (
            <Empty className='min-h-[300px]'>
              <EmptyHeader>
                <EmptyMedia variant='icon'>
                  <HugeiconsIcon
                    className='size-6'
                    icon={Database01Icon}
                    strokeWidth={2}
                  />
                </EmptyMedia>
                <EmptyTitle>{emptyTitle ?? t('table.empty')}</EmptyTitle>
                {emptyDescription && (
                  <EmptyDescription>{emptyDescription}</EmptyDescription>
                )}
              </EmptyHeader>
              {emptyAction && <EmptyContent>{emptyAction}</EmptyContent>}
            </Empty>
          ) : (
            data.map((item, index) => (
              <div key={table.getRowModel().rows[index]?.id ?? index}>
                {renderMobileCard(item)}
              </div>
            ))
          )}
        </div>
      )}
      {pagination}
    </div>
  )
}
