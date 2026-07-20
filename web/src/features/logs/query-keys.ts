function stableLogParams(params: object) {
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

export const logKeys = {
  all: ['logs'] as const,
  global: (params: object) =>
    ['logs', 'global', stableLogParams(params)] as const,
  site: (siteId: string, params: object) =>
    ['logs', 'site', siteId, stableLogParams(params)] as const,
}
