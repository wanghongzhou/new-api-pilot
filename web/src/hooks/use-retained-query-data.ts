import { useRef } from 'react'

export function useRetainedQueryData<T>(
  data: T | undefined,
  isError: boolean,
  scope = 'default'
): T | undefined {
  const retained = useRef<{ data: T; scope: string } | undefined>(undefined)
  if (data !== undefined) retained.current = { data, scope }
  return (
    data ??
    (isError && retained.current?.scope === scope
      ? retained.current.data
      : undefined)
  )
}
