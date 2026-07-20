import type { AccountListParams, RemoteUserListParams } from './types'

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

export const accountKeys = {
  all: ['accounts'] as const,
  lists: () => ['accounts', 'list'] as const,
  list: (params: AccountListParams) =>
    ['accounts', 'list', stableParams(params)] as const,
  detail: (id: string) => ['accounts', 'detail', id] as const,
  remoteUsers: (siteId: string, params: RemoteUserListParams) =>
    ['accounts', 'remote-users', siteId, stableParams(params)] as const,
  statistics: (id: string, params: object) =>
    ['accounts', 'statistics', id, stableParams(params)] as const,
}
