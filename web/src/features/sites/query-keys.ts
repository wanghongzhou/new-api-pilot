import type {
  CollectionRunListParams,
  CollectionRunWindowListParams,
  FastTaskHistoryListParams,
  SiteListParams,
  SiteResourceQuery,
} from './types'

type QueryValue = string | number | readonly string[] | undefined

export function stableSiteParams<T extends object>(
  params: T
): Record<string, string | number | string[]> {
  return Object.fromEntries(
    (Object.entries(params) as [string, QueryValue][])
      .filter(
        (entry): entry is [string, Exclude<QueryValue, undefined>] =>
          entry[1] !== undefined
      )
      .map(([key, value]): [string, string | number | string[]] => [
        key,
        Array.isArray(value) ? [...value].sort() : (value as string | number),
      ])
      .sort((left, right) => left[0].localeCompare(right[0]))
  )
}

export const siteKeys = {
  all: ['sites'] as const,
  lists: () => ['sites', 'list'] as const,
  list: (params: SiteListParams) =>
    ['sites', 'list', stableSiteParams(params)] as const,
  detail: (id: string) => ['sites', 'detail', id] as const,
  performance: (id: string, hours: number) =>
    ['sites', 'performance', id, hours] as const,
  statistics: (id: string, params: object) =>
    ['sites', 'statistics', id, stableSiteParams(params)] as const,
  status: (id: string, params: SiteResourceQuery) =>
    ['sites', 'status', id, stableSiteParams(params)] as const,
  instances: (id: string) => ['sites', 'instances', id] as const,
  runs: (id: string, params: CollectionRunListParams) =>
    ['sites', 'runs', id, stableSiteParams(params)] as const,
  run: (id: string) => ['sites', 'run', id] as const,
  windows: (id: string, params: CollectionRunWindowListParams) =>
    ['sites', 'run', id, 'windows', stableSiteParams(params)] as const,
  fastTaskHistory: (id: string, params: FastTaskHistoryListParams) =>
    ['sites', 'fast-task-history', id, stableSiteParams(params)] as const,
}
