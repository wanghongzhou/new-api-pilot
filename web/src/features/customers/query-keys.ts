import type { CustomerListParams } from './types'

type QueryValue = string | number | readonly string[] | undefined

function stableParams(values: object) {
  return Object.fromEntries(
    (Object.entries(values) as [string, QueryValue][])
      .filter((entry) => entry[1] !== undefined)
      .map(
        ([key, value]) =>
          [key, Array.isArray(value) ? [...value].sort() : value] as const
      )
      .sort((left, right) => left[0].localeCompare(right[0]))
  )
}

export const customerKeys = {
  all: ['customers'] as const,
  lists: () => ['customers', 'list'] as const,
  list: (params: CustomerListParams) =>
    ['customers', 'list', stableParams(params)] as const,
  detail: (id: string) => ['customers', 'detail', id] as const,
  accounts: (id: string, page: number) =>
    ['customers', 'detail', id, 'accounts', page] as const,
  statistics: (id: string, params: object) =>
    ['customers', 'statistics', id, stableParams(params)] as const,
}
