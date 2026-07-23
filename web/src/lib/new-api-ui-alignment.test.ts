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

test('site management filters and onboarding follow the responsive drawer layout', async () => {
  const filtersSource = await readFile(
    new URL('../features/sites/components/site-filters.tsx', import.meta.url),
    'utf8'
  )
  const drawerSource = await readFile(
    new URL(
      '../features/sites/components/site-onboarding-drawer.tsx',
      import.meta.url
    ),
    'utf8'
  )

  expect(filtersSource).toContain('renderGroups(filterGroups)')
  expect(filtersSource).not.toContain('advancedGroups')
  expect(filtersSource).toContain("t('site.filters.statusTitle'")
  expect(filtersSource).toContain('<FacetedFilter')
  expect(filtersSource).toContain('hasActiveFilters={hasActiveFilters}')
  expect(drawerSource).toContain('sideDrawerContentClassName')
  expect(drawerSource).toContain('sideDrawerHeaderClassName')
  expect(drawerSource).toContain('sideDrawerFormClassName')
  expect(drawerSource).toContain('sideDrawerFooterClassName')
})

test('shared search filters use search actions and only show reset for active filters', async () => {
  const source = await readFile(
    new URL('../components/data/filter-panel.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain("t('common.search')")
  expect(source).not.toContain("t('common.apply')")
  expect(source).toContain('(hasActiveFilters ?? true)')
  expect(source).toContain('{actions}')
})

test('site status filters use concise status titles', async () => {
  const locale = await readFile(
    new URL('../i18n/locales/zh-CN.json', import.meta.url),
    'utf8'
  )

  expect(locale).toContain('"site.filters.statusTitle": "{{status}}状态"')
  expect(locale).not.toContain('"site.filters.allStatus"')
})

test('site card and table views share the same empty-state presentation', async () => {
  const source = await readFile(
    new URL('../features/sites/components/sites-page.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain(
    "<Empty className='border-border bg-background min-h-64 border'>"
  )
  expect(source).toContain('icon={Database01Icon}')
  expect(source).toContain('icon={Alert02Icon}')
  expect(source).toContain('<PageFooterPortal>')
  expect(source).not.toContain('bg-card rounded-lg border p-8 text-center')
})

test('site view controls follow the new-api channel toolbar pattern', async () => {
  const source = await readFile(
    new URL('../features/sites/components/sites-page.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain('actions={')
  expect(source).toContain('<DataViewModeToggle')
  expect(source).not.toContain("<div className='flex justify-end'>")
})

test('site filters auto-search and faceted menus close on outside clicks', async () => {
  const filtersSource = await readFile(
    new URL('../features/sites/components/site-filters.tsx', import.meta.url),
    'utf8'
  )
  const facetedSource = await readFile(
    new URL('../components/data/faceted-filter.tsx', import.meta.url),
    'utf8'
  )

  expect(filtersSource).toContain('applyImmediately')
  expect(filtersSource).toContain('searchTimerRef.current = setTimeout')
  expect(filtersSource).not.toContain('onApply={() =>')
  expect(facetedSource).toContain("document.addEventListener('pointerdown'")
  expect(facetedSource).toContain("details.removeAttribute('open')")
})

test('site list keeps its table header when no rows are available', async () => {
  const pageSource = await readFile(
    new URL('../features/sites/components/sites-page.tsx', import.meta.url),
    'utf8'
  )
  const tableSource = await readFile(
    new URL('../components/ui/data-table.tsx', import.meta.url),
    'utf8'
  )

  expect(pageSource).toContain('preserveHeaderWhenEmpty')
  expect(pageSource).toContain('fillAvailableHeight')
  expect(pageSource).toContain('fixedContent')
  expect(pageSource).not.toContain('emptyAction={')
  expect(tableSource).toContain('emptyTableBody')
  expect(tableSource).toContain('table.getVisibleLeafColumns().length')
})

test('site empty states do not render create actions', async () => {
  const source = await readFile(
    new URL('../features/sites/components/sites-page.tsx', import.meta.url),
    'utf8'
  )

  const cardState = source.slice(
    source.indexOf('function CardGridState'),
    source.indexOf('export function SitesPage')
  )
  expect(cardState).not.toContain('onCreate')
  expect(cardState).not.toContain("t('sites.create')")
})

test('platform users follow the new-api user list layout without a view toggle', async () => {
  const pageSource = await readFile(
    new URL(
      '../features/platform-users/components/platform-users-page.tsx',
      import.meta.url
    ),
    'utf8'
  )
  const filtersSource = await readFile(
    new URL(
      '../features/platform-users/components/platform-user-filters.tsx',
      import.meta.url
    ),
    'utf8'
  )

  expect(pageSource).toContain('fixedContent')
  expect(pageSource).toContain('fillAvailableHeight')
  expect(pageSource).toContain('preserveHeaderWhenEmpty')
  expect(pageSource).toContain('<PageFooterPortal>')
  expect(pageSource).not.toContain('DataViewModeToggle')
  expect(pageSource).not.toContain('Refresh01Icon')
  expect(pageSource).not.toContain("t('common.refresh')")
  expect(filtersSource).toContain('<FacetedFilter')
  expect(filtersSource).toContain('searchTimerRef.current = setTimeout')
  expect(filtersSource).toContain('hasActiveFilters={hasActiveFilters}')
  expect(filtersSource).not.toContain('<Select')
  expect(filtersSource).not.toContain('onApply={() =>')
  expect(filtersSource).not.toContain("t('All roles')")
  expect(filtersSource).not.toContain("t('All statuses')")
})

test('entity create and edit flows use right-side drawers while short tasks keep dialogs', async () => {
  const platformUsersSource = await readFile(
    new URL(
      '../features/platform-users/components/user-dialogs.tsx',
      import.meta.url
    ),
    'utf8'
  )
  const customersSource = await readFile(
    new URL(
      '../features/customers/components/customer-dialogs.tsx',
      import.meta.url
    ),
    'utf8'
  )
  const accountsSource = await readFile(
    new URL(
      '../features/accounts/components/account-dialogs.tsx',
      import.meta.url
    ),
    'utf8'
  )
  const sitesSource = await readFile(
    new URL('../features/sites/components/site-dialogs.tsx', import.meta.url),
    'utf8'
  )
  const alertRulesSource = await readFile(
    new URL(
      '../features/alerts/components/alert-rule-dialogs.tsx',
      import.meta.url
    ),
    'utf8'
  )

  for (const source of [
    platformUsersSource,
    customersSource,
    accountsSource,
    sitesSource,
    alertRulesSource,
  ]) {
    expect(source).toContain("<Drawer direction='right'")
    expect(source).toContain('sideDrawerContentClassName')
    expect(source).toContain('sideDrawerHeaderClassName')
    expect(source).toContain('sideDrawerFormClassName')
    expect(source).toContain('sideDrawerFooterClassName')
  }

  expect(platformUsersSource).toContain('function ResetPasswordDialog')
  expect(platformUsersSource).toContain('<Dialog onOpenChange={onOpenChange}')
  expect(
    platformUsersSource.match(/alignItemWithTrigger=\{false\}/g)
  ).toHaveLength(2)
  expect(platformUsersSource.match(/portalled=\{false\}/g)).toHaveLength(2)
  expect(customersSource).toContain('<ConfirmDialog')
  expect(accountsSource).toContain('<ConfirmDialog')
  expect(sitesSource).toContain('function AuthorizationDialog')
  expect(sitesSource).toContain('<Dialog onOpenChange=')
  expect(alertRulesSource).toContain('<ConfirmDialog')
})

test('the browser tab uses the project favicon', async () => {
  const html = await Bun.file(
    new URL('../../index.html', import.meta.url)
  ).text()
  const favicon = await Bun.file(
    new URL('../../public/favicon.svg', import.meta.url)
  ).text()

  expect(html).toContain('rel="icon"')
  expect(html).toContain('href="/favicon.svg"')
  expect(favicon).toContain('<svg')
  expect(favicon).toContain('fill="oklch(0.692 0.141 243.716)"')
  expect(favicon).toContain('M11 5h7')
  expect(favicon).toContain('cx="6.44444"')
})

test('the default theme keeps the new-api color tokens', async () => {
  const theme = await readFile(
    new URL('../styles/theme.css', import.meta.url),
    'utf8'
  )

  expect(theme).toContain('--background: oklch(1 0 0)')
  expect(theme).toContain('--foreground: oklch(0.145 0 0)')
  expect(theme).toContain('--primary: oklch(0.692 0.141 243.716)')
  expect(theme).toContain('--secondary: oklch(0.95 0 0)')
  expect(theme).toContain('--border: oklch(0.93 0 0)')
  expect(theme).toContain('--ring: oklch(0.708 0.16 249.003)')
  expect(theme).toContain('--sidebar-primary: oklch(0.64 0.197 253.892)')
})
