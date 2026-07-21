import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}

export function getPageNumbers(currentPage: number, totalPages: number) {
  const maxVisiblePages = 4
  const rangeWithDots: Array<number | '...'> = []

  if (totalPages <= maxVisiblePages) {
    for (let page = 1; page <= totalPages; page++) {
      rangeWithDots.push(page)
    }
  } else {
    rangeWithDots.push(1)

    if (currentPage <= 2) {
      rangeWithDots.push(2, '...', totalPages)
    } else if (currentPage >= totalPages - 1) {
      rangeWithDots.push('...', totalPages - 1, totalPages)
    } else {
      rangeWithDots.push('...', currentPage, '...', totalPages)
    }
  }

  return rangeWithDots
}
