import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

describe('application shell navigation contract', () => {
  test('focuses main content after pathname navigation without stealing initial focus', async () => {
    const source = await readFile(
      new URL('../page-transition.tsx', import.meta.url),
      'utf8'
    )

    expect(source).toContain('initialPathname')
    expect(source).toContain(
      "querySelector<HTMLElement>('#main-content')?.focus()"
    )
    expect(source).toContain('[pathname]')
  })

  test('keeps theme preferences reachable on mobile without dangling descriptions', async () => {
    const source = await readFile(
      new URL('./theme-settings-drawer.tsx', import.meta.url),
      'utf8'
    )
    const trigger = source.slice(
      source.indexOf('<SheetTrigger'),
      source.indexOf('</SheetTrigger>')
    )

    expect(trigger).toContain("className='size-10 sm:size-8'")
    expect(trigger).not.toContain('max-md:hidden')
    expect(trigger).not.toContain(
      "aria-describedby='config-drawer-description'"
    )
  })
})
