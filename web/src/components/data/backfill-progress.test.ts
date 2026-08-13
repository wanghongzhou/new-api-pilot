import { expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import zhCN from '@/i18n/locales/zh-CN.json'

test('failed backfills remain visibly failed even at terminal progress', async () => {
  const source = await readFile(
    new URL('./backfill-progress.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain("backfill.status === 'failed'")
  expect(source).toContain("dynamicI18nKey('backfill', statusKey)")
  expect(source).toContain('backfill.failed_windows')
  expect(zhCN['backfill.status.failed']).toBe('失败')
  expect(zhCN['backfill.status.pending']).toBe('等待中')
  expect(zhCN['backfill.status.running']).toBe('进行中')
})
