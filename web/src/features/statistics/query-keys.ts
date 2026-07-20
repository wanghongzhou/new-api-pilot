export const statisticsKeys = {
  all: ['statistics'] as const,
  scope: (scope: string, params: object) =>
    ['statistics', 'scope', scope, stableStatisticsParams(params)] as const,
  options: (
    type: 'channels' | 'models' | 'groups' | 'tokens' | 'nodes',
    params: object
  ) => ['statistics', 'options', type, stableStatisticsParams(params)] as const,
  exports: () => ['statistics', 'exports'] as const,
  exportLists: () => ['statistics', 'exports', 'list'] as const,
  exportList: (params: object) =>
    ['statistics', 'exports', 'list', stableStatisticsParams(params)] as const,
  export: (id: string) => ['statistics', 'exports', 'detail', id] as const,
}

function stableStatisticsParams(params: object) {
  return Object.fromEntries(
    Object.entries(params)
      .filter(([, value]) => value !== undefined)
      .map(([key, value]) => [
        key,
        Array.isArray(value) ? [...value].sort() : value,
      ])
      .sort(([left], [right]) => left.localeCompare(right))
  )
}
