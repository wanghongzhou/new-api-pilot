function stableParams(params: object) {
  return Object.fromEntries(
    Object.entries(params)
      .filter(([, value]) => value !== undefined && value !== '')
      .map(([key, value]) => [
        key,
        Array.isArray(value) ? [...value].sort() : value,
      ])
      .sort(([left], [right]) => left.localeCompare(right))
  )
}

export const upstreamTaskKeys = {
  all: ['upstream-tasks'] as const,
  globalList: (params: object) =>
    ['upstream-tasks', 'global', 'list', stableParams(params)] as const,
  globalStatistics: (params: object) =>
    ['upstream-tasks', 'global', 'statistics', stableParams(params)] as const,
  siteList: (siteId: string, params: object) =>
    ['upstream-tasks', 'site', siteId, 'list', stableParams(params)] as const,
  siteStatistics: (siteId: string, params: object) =>
    [
      'upstream-tasks',
      'site',
      siteId,
      'statistics',
      stableParams(params),
    ] as const,
}
