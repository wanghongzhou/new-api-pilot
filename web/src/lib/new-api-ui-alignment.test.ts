import { expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import { getUserAvatarFallback, getUserAvatarStyle } from './avatar'
import { getPageNumbers } from './utils'

test('shared Select remains the Base UI primitive instead of a native alias', async () => {
  const source = await readFile(
    new URL('../components/ui/select.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain("from '@base-ui/react/select'")
  expect(source).not.toContain('NativeSelect as Select')
})

test('shared pagination keeps the reference numbered-page contract', async () => {
  expect(getPageNumbers(1, 4)).toEqual([1, 2, 3, 4])
  expect(getPageNumbers(1, 10)).toEqual([1, 2, '...', 10])
  expect(getPageNumbers(5, 10)).toEqual([1, '...', 5, '...', 10])
  expect(getPageNumbers(10, 10)).toEqual([1, '...', 9, 10])

  const source = await readFile(
    new URL('../components/ui/data-table-pagination.tsx', import.meta.url),
    'utf8'
  )
  expect(source).toContain('table.rowsPerPage')
  expect(source).toContain("aria-label={t('table.rowsPerPage')}")
  expect(source).not.toContain('text-muted-foreground/80')
  expect(source).toContain('ArrowLeftDoubleIcon')
  expect(source).toContain('ArrowRightDoubleIcon')
})

test('profile avatar uses the reference stable fallback and color model', async () => {
  expect(getUserAvatarFallback('operator')).toBe('O')
  expect(getUserAvatarFallback('')).toBe('?')
  expect(getUserAvatarStyle('operator')).toEqual(getUserAvatarStyle('operator'))
  expect(getUserAvatarStyle('operator')).not.toEqual(
    getUserAvatarStyle('administrator')
  )

  const source = await readFile(
    new URL('../components/layout/app-header.tsx', import.meta.url),
    'utf8'
  )
  expect(source).toContain("from '../ui/avatar'")
  expect(source).toContain('getUserAvatarStyle')
})
