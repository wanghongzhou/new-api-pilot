import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

describe('alert delivery diagnostics privacy boundary', () => {
  test('defensively redacts credentials, URLs and webhook diagnostics', async () => {
    const source = await readFile(
      new URL('./components/alert-event-detail-sheet.tsx', import.meta.url),
      'utf8'
    )

    for (const marker of [
      'access_token',
      'authorization',
      'password',
      'secret',
      'webhook',
      'https://',
    ]) {
      expect(source).toContain(`'${marker}'`)
    }
    expect(source).toContain("? '[redacted]'")
    expect(source).not.toContain('{delivery.response_message}')
  })
})
