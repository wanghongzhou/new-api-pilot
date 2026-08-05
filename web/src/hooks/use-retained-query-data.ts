import { useRef } from 'react'

export function useRetainedQueryData<T>(
  data: T | undefined,
  isError: boolean
): T | undefined {
  const lastSuccessfulData = useRef<T>(undefined)
  if (data !== undefined) lastSuccessfulData.current = data
  return data ?? (isError ? lastSuccessfulData.current : undefined)
}
