import { expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

test('uses a non-skipping default heading level for page and table empty states', async () => {
  const source = await readFile(new URL('./empty.tsx', import.meta.url), 'utf8')

  expect(source).toContain("React.ComponentProps<'h2'>")
  expect(source).toContain('<h2')
  expect(source).not.toContain("React.ComponentProps<'h3'>")
})
