export const rankingKeys = {
  global: (tab: string, params: object) =>
    ['rankings', 'global', tab, params] as const,
  site: (siteId: string, tab: string, params: object) =>
    ['rankings', 'site', siteId, tab, params] as const,
}
