import { expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

test('zero expected completeness is presented as not evaluable', async () => {
  const source = await readFile(
    new URL('./completeness-alert.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain('completeness.expected_unit_count > 0')
  expect(source).toContain("t('completeness.notEvaluable')")
  expect(source).toContain("t('completeness.scopeDescription')")
  expect(source).toContain(
    'evaluable && <DataStatusBadge status={completeness.data_status} />'
  )
})
