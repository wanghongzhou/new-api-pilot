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

  const controlSource = await readFile(
    new URL('../components/ui/select-control.tsx', import.meta.url),
    'utf8'
  )
  expect(controlSource).toContain("from '@/components/ui/select'")
  expect(controlSource).not.toContain('<select')
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

test('theme settings keep the official config drawer structure', async () => {
  const drawerSource = await readFile(
    new URL('../components/layout/theme-settings-drawer.tsx', import.meta.url),
    'utf8'
  )
  const rootSource = await readFile(
    new URL('../routes/__root.tsx', import.meta.url),
    'utf8'
  )
  const globalStyles = await readFile(
    new URL('../styles/index.css', import.meta.url),
    'utf8'
  )

  expect(drawerSource).toContain("from '@base-ui/react/radio'")
  expect(drawerSource).toContain("from '@base-ui/react/radio-group'")
  expect(drawerSource).toContain('useDirection')
  expect(drawerSource).toContain('sideDrawerContentClassName')
  expect(drawerSource).toContain('function SectionTitle')
  expect(drawerSource).toContain('<RotateCcw')
  expect(rootSource).toContain('<DirectionProvider>')
  expect(rootSource).toContain('<Toaster closeButton />')
  expect(globalStyles).toContain('@media (pointer: coarse)')
  expect(globalStyles).toContain('min-height: 2.5rem')
})
