import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import { safeRedirect } from '../safe-redirect'

describe('authentication page boundaries', () => {
  test('keeps post-login redirects same-origin', () => {
    expect(safeRedirect('/dashboard?range=today#health')).toBe(
      '/dashboard?range=today#health'
    )
    expect(safeRedirect('//evil.example/path')).toBe('/dashboard')
    expect(safeRedirect('/\\evil.example/path')).toBe('/dashboard')
    expect(safeRedirect('https://evil.example/path')).toBe('/dashboard')
    expect(safeRedirect(undefined)).toBe('/dashboard')
  })

  test('uses one main landmark without forcing nested viewport height', async () => {
    const [layout, signIn, changePassword] = await Promise.all([
      readFile(new URL('./auth-layout.tsx', import.meta.url), 'utf8'),
      readFile(new URL('./sign-in-page.tsx', import.meta.url), 'utf8'),
      readFile(new URL('./change-password-page.tsx', import.meta.url), 'utf8'),
    ])

    expect(layout).toContain('<main')
    expect(layout).toContain("id='main-content'")
    expect(layout).not.toContain("const Root = standalone ? 'main' : 'div'")
    expect(layout).toContain("'relative grid min-h-full max-w-none'")
    expect(signIn).toContain('<AuthLayout standalone>')
    expect(changePassword).toContain('<AuthLayout>')
  })
})
