export function safeRedirect(redirect: string | undefined): string {
  if (!redirect || !redirect.startsWith('/') || redirect.includes('\\')) {
    return '/dashboard'
  }
  try {
    const base = 'https://new-api-pilot.invalid'
    const destination = new URL(redirect, base)
    if (destination.origin !== base) return '/dashboard'
    return `${destination.pathname}${destination.search}${destination.hash}`
  } catch {
    return '/dashboard'
  }
}
