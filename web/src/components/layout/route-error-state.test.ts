import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

import { SessionVerificationError } from '@/features/auth/session-verification-error'

describe('route error boundaries', () => {
  const rootRoute = readFileSync(
    new URL('../../routes/__root.tsx', import.meta.url),
    'utf8'
  )
  const authenticatedRoute = readFileSync(
    new URL('../../routes/_authenticated/route.tsx', import.meta.url),
    'utf8'
  )
  const authBoundary = readFileSync(
    new URL(
      '../../features/auth/components/auth-boundary-state.tsx',
      import.meta.url
    ),
    'utf8'
  )
  const routeBoundary = readFileSync(
    new URL('./route-error-state.tsx', import.meta.url),
    'utf8'
  )
  const locale = JSON.parse(
    readFileSync(
      new URL('../../i18n/locales/zh-CN.json', import.meta.url),
      'utf8'
    )
  ) as Record<string, string>

  test('keeps session verification failures on the dedicated auth state', () => {
    const originalError = new Error('upstream unavailable')
    const verificationError = new SessionVerificationError(originalError)

    expect(verificationError.originalError).toBe(originalError)
    expect(verificationError.name).toBe('SessionVerificationError')
    expect(authenticatedRoute).toContain(
      'throw new SessionVerificationError(apiError)'
    )
    expect(authenticatedRoute).toContain(
      'errorComponent: AuthenticatedRouteErrorState'
    )
    expect(authBoundary).toContain(
      'props.error instanceof SessionVerificationError'
    )
    expect(authBoundary).toContain('<AuthErrorState')
    expect(authBoundary).toContain('<RouteErrorState')
  })

  test('provides root error and not-found fallbacks with recovery actions', () => {
    expect(rootRoute).toContain('errorComponent: RouteErrorState')
    expect(rootRoute).toContain('notFoundComponent: RouteNotFoundState')
    expect(routeBoundary).toContain("t('Back to dashboard')")
    expect(routeBoundary).toContain("t('Retry')")
    expect(routeBoundary).toContain("t('Page not found')")
    expect(routeBoundary).toContain("to='/dashboard'")
  })

  test('ships concise Chinese copy for generic errors and unknown routes', () => {
    expect(locale['Page failed to load']).toBe('页面加载失败')
    expect(locale['Back to dashboard']).toBe('返回工作台')
    expect(locale['Page not found']).toBe('页面不存在')
    expect(
      locale['The page you requested does not exist or has been moved.']
    ).toBe('你访问的页面不存在或已被移动。')
  })
})
