import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

describe('system task frontend is passive and uses no private task fields', () => {
  test('contains only list/statistics GETs and safe DTO fields', async () => {
    const source = (
      await Promise.all(
        ['types.ts', 'api.ts', 'components/system-tasks-page.tsx'].map((file) =>
          readFile(new URL(file, import.meta.url), 'utf8')
        )
      )
    )
      .join('\n')
      .toLowerCase()
    for (const field of [
      ['active', 'key'].join('_'),
      ['locked', 'by'].join('_'),
      ['raw', 'json'].join('_'),
      ['error', 'message'].join('_'),
      ['access', 'token'].join('_'),
      ['authorization'].join('_'),
      ['credential'].join('_'),
      ['payload'].join('_'),
      ['private', 'data'].join('_'),
    ]) {
      expect(source).not.toContain(field)
    }
    expect(source).not.toMatch(/method:\s*['"](?:post|put|patch|delete)['"]/)
    expect(source).not.toMatch(/system-tasks\/\$\{|system-tasks\/\d/)
  })
})
