import { expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

test('uses distinct semantic icons for site card destinations', async () => {
  const source = await readFile(
    new URL('./site-card.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain('icon={Chart01Icon}')
  expect(source).toContain('icon={ServerStack01Icon}')
  expect(source).toContain('icon={ViewIcon}')
  expect(source).not.toContain('ArrowRight01Icon')
  expect(source).toContain("toast.success(t('site.toast.baseUrlCopied'))")
  expect(source).toContain("toast.error(t('site.toast.copyFailed'))")
})
