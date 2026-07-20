function stableInventoryParams(params: object) {
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

export const userInventoryKeys = {
  all: ['user-inventory'] as const,
  globalList: (params: object) =>
    [
      'user-inventory',
      'global',
      'list',
      stableInventoryParams(params),
    ] as const,
  globalStatistics: (params: object) =>
    [
      'user-inventory',
      'global',
      'statistics',
      stableInventoryParams(params),
    ] as const,
  siteList: (siteId: string, params: object) =>
    [
      'user-inventory',
      'site',
      siteId,
      'list',
      stableInventoryParams(params),
    ] as const,
  siteStatistics: (siteId: string, params: object) =>
    [
      'user-inventory',
      'site',
      siteId,
      'statistics',
      stableInventoryParams(params),
    ] as const,
}
