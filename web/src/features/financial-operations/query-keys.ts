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

export const financialOperationsKeys = {
  all: ['financial-operations'] as const,
  list: (kind: string, siteId: string | undefined, params: object) =>
    [
      'financial-operations',
      kind,
      siteId ?? 'global',
      'list',
      stableParams(params),
    ] as const,
  statistics: (kind: string, siteId: string | undefined, params: object) =>
    [
      'financial-operations',
      kind,
      siteId ?? 'global',
      'statistics',
      stableParams(params),
    ] as const,
}
