import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

describe('logs mobile layout contract', () => {
  test('keeps the detail footer visible while the body scrolls', async () => {
    const source = await readFile(
      new URL('components/logs-page.tsx', import.meta.url),
      'utf8'
    )

    expect(source).toContain('max-h-[calc(100dvh-2rem)] max-w-3xl')
    expect(source).toContain(
      "className='grid min-h-0 gap-4 overflow-y-auto pr-1'"
    )
    expect(source).toMatch(
      /overflow-y-auto pr-1'[\s\S]*<\/div>\s*<DialogFooter>/
    )
  })
})
