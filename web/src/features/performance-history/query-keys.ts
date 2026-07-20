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

export const performanceHistoryKeys = {
  all: ['performance-history'] as const,
  globalList: (params: object) =>
    ['performance-history', 'global', 'list', stableParams(params)] as const,
  globalStatistics: (params: object) =>
    [
      'performance-history',
      'global',
      'statistics',
      stableParams(params),
    ] as const,
  siteList: (siteId: string, params: object) =>
    [
      'performance-history',
      'site',
      siteId,
      'list',
      stableParams(params),
    ] as const,
  siteStatistics: (siteId: string, params: object) =>
    [
      'performance-history',
      'site',
      siteId,
      'statistics',
      stableParams(params),
    ] as const,
}
