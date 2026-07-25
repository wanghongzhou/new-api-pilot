import type { IdString } from '@/lib/api-types'

const defaultRefreshConcurrency = 4

export async function refreshAccountsBounded(
  accountIds: readonly IdString[],
  refresh: (accountId: IdString) => Promise<unknown>,
  concurrency = defaultRefreshConcurrency
) {
  const normalizedConcurrency = Number.isFinite(concurrency)
    ? Math.max(1, Math.trunc(concurrency))
    : defaultRefreshConcurrency
  const workerCount = Math.min(accountIds.length, normalizedConcurrency)
  let nextIndex = 0
  let failed = 0
  const workers = Array.from({ length: workerCount }, async () => {
    while (nextIndex < accountIds.length) {
      const accountId = accountIds[nextIndex]
      nextIndex += 1
      try {
        await refresh(accountId)
      } catch {
        failed += 1
      }
    }
  })
  await Promise.all(workers)
  return { failed, total: accountIds.length }
}
