export function requiresDocumentNavigation(pathname: string): boolean {
  const normalizedPathname = pathname.replace(/\/+$/, '') || '/'
  return normalizedPathname.endsWith('/pricing-groups')
}
