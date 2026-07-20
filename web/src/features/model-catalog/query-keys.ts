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

export const modelCatalogKeys = {
  all: ['model-catalog'] as const,
  global: (kind: string, params: object) =>
    ['model-catalog', 'global', kind, stableParams(params)] as const,
  site: (siteId: string, kind: string, params: object) =>
    ['model-catalog', 'site', siteId, kind, stableParams(params)] as const,
}
