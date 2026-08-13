import { useEffect } from 'react'

export function lastValidPage(total: number, pageSize: number): number {
  if (total <= 0 || pageSize <= 0) return 1
  return Math.max(1, Math.ceil(total / pageSize))
}

export function pageReplacement({
  isFetching,
  isPlaceholderData,
  page,
  pageSize,
  total,
}: {
  isFetching: boolean
  isPlaceholderData: boolean
  page: number
  pageSize: number
  total: number | string | undefined
}): number | undefined {
  if (total == null || isFetching || isPlaceholderData) return undefined
  if (typeof total === 'string') {
    if (total === '0') return page > 1 ? 1 : undefined
    const totalPages =
      (BigInt(total) + BigInt(pageSize) - 1n) / BigInt(pageSize)
    const firstOffset = BigInt(page - 1) * BigInt(pageSize)
    if (page <= 1 || firstOffset < BigInt(total)) return undefined
    return totalPages <= BigInt(Number.MAX_SAFE_INTEGER)
      ? Number(totalPages)
      : page - 1
  }
  const validPage = lastValidPage(total, pageSize)
  return page > validPage ? validPage : undefined
}

export function useLastValidPage({
  isFetching,
  isPlaceholderData,
  onReplace,
  page,
  pageSize,
  total,
}: {
  isFetching: boolean
  isPlaceholderData: boolean
  onReplace: (page: number) => void
  page: number
  pageSize: number
  total: number | string | undefined
}) {
  useEffect(() => {
    const replacement = pageReplacement({
      isFetching,
      isPlaceholderData,
      page,
      pageSize,
      total,
    })
    if (replacement != null) onReplace(replacement)
  }, [isFetching, isPlaceholderData, onReplace, page, pageSize, total])
}
