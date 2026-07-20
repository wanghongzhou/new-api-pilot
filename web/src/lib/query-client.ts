import { QueryClient } from '@tanstack/react-query'

import { isRetryableApiError } from './api'

const maximumQueryRetries = 2

export function shouldRetryQuery(
  failureCount: number,
  error: unknown
): boolean {
  return failureCount < maximumQueryRetries && isRetryableApiError(error)
}

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      gcTime: 30 * 60 * 1000,
      refetchOnReconnect: true,
      refetchOnWindowFocus: false,
      retry: shouldRetryQuery,
      retryDelay: (attemptIndex) => Math.min(1000 * 2 ** attemptIndex, 5000),
      staleTime: 10_000,
    },
    mutations: {
      retry: false,
    },
  },
})
