import type { SystemTaskQueryParams } from './types'

export const systemTaskKeys = {
  all: ['system-tasks'] as const,
  global: (kind: 'list' | 'statistics', params: SystemTaskQueryParams) =>
    [...systemTaskKeys.all, 'global', kind, params] as const,
  site: (
    siteId: string,
    kind: 'list' | 'statistics',
    params: SystemTaskQueryParams
  ) => [...systemTaskKeys.all, 'site', siteId, kind, params] as const,
}
