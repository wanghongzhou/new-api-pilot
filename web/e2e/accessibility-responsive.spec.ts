import { readdirSync } from 'node:fs'
import { dirname, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type TestInfo } from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'
const routeRoot = resolve(
  dirname(fileURLToPath(import.meta.url)),
  '../src/routes'
)

const admin = {
  display_name: '验收管理员',
  id: '9007199254740993',
  must_change_password: false,
  role: 'admin',
  status: 1,
  username: 'acceptance-admin',
} as const

function envelope<T>(data: T) {
  return {
    code: '',
    data,
    message: '',
    request_id: 'req_a77_self',
    success: true,
  }
}

function failureEnvelope(pathname: string) {
  return {
    code: 'VALIDATION_ERROR',
    data: null,
    field_errors: {},
    message: 'acceptance route fallback',
    request_id: `req_a77_${pathname.replaceAll(/[^a-z0-9]+/gi, '_')}`,
    success: false,
  }
}

function routeFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return routeFiles(path)
    if (entry.isFile() && entry.name.endsWith('.tsx')) return [path]
    return []
  })
}

function routePath(file: string): string | null {
  const routeFile = relative(routeRoot, file)
  const parts = routeFile.split(sep)
  const filename = parts.pop()?.replace(/\.tsx$/, '')
  if (!filename || filename === '__root' || filename === 'route') return null
  if (filename !== 'index') parts.push(filename)
  const segments = parts
    .filter(
      (part) =>
        !part.startsWith('_') && !(part.startsWith('(') && part.endsWith(')'))
    )
    .map((part) => (part.startsWith('$') ? '1' : part))
  return `/${segments.join('/')}`.replace(/\/$/, '') || '/'
}

const allRoutes = [
  ...new Set(
    routeFiles(routeRoot)
      .map(routePath)
      .filter((path): path is string => path !== null)
  ),
].sort()
const authenticatedRoutes = allRoutes.filter((path) => path !== '/sign-in')

async function installAuthenticatedFailureBoundary(page: Page) {
  await page.addInitScript(
    ({ authKey, authUser, uidKey }) => {
      window.localStorage.setItem(authKey, JSON.stringify(authUser))
      window.localStorage.setItem(uidKey, authUser.id)
    },
    { authKey: authStorageKey, authUser: admin, uidKey: uidStorageKey }
  )
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url())
    if (url.pathname === '/api/user/self') {
      await route.fulfill({ json: envelope(admin) })
      return
    }
    await route.fulfill({
      json: failureEnvelope(url.pathname),
      status: 400,
    })
  })
}

async function hideDeveloperOverlays(page: Page) {
  await page.addStyleTag({
    content: `
      button[aria-label='Open TanStack Router Devtools'],
      button[aria-label='Open Tanstack query devtools'] {
        display: none !important;
      }
    `,
  })
}

async function assertKeyboardFocus(page: Page) {
  await page.keyboard.press('Tab')
  const focused = await page.evaluate(() => {
    const element = document.activeElement
    if (!(element instanceof HTMLElement) || element === document.body) {
      return null
    }
    const rectangle = element.getBoundingClientRect()
    const style = getComputedStyle(element)
    return {
      height: rectangle.height,
      tag: element.tagName,
      visibility: style.visibility,
      width: rectangle.width,
    }
  })
  expect(focused).not.toBeNull()
  expect(focused?.visibility).not.toBe('hidden')
  expect((focused?.height ?? 0) > 0 && (focused?.width ?? 0) > 0).toBe(true)
}

async function assertNoAccessibilityViolations(page: Page, route: string) {
  const result = await new AxeBuilder({ page })
    .exclude("button[aria-label='Open TanStack Router Devtools']")
    .exclude("button[aria-label='Open Tanstack query devtools']")
    .analyze()
  expect(
    result.violations.map(({ id, nodes }) => ({
      id,
      nodes: nodes.map(({ failureSummary, html, target }) => ({
        failureSummary,
        html,
        target,
      })),
    })),
    `axe violations on ${route}`
  ).toEqual([])
}

async function assertTouchTargets(page: Page, route: string) {
  const undersized = await page
    .locator(
      "button:not([disabled]), a[href], input:not([type='hidden']):not([aria-hidden='true']):not([disabled]), select:not([disabled]), textarea:not([disabled]), [role='button']:not([aria-disabled='true']), [role='tab']:not([aria-disabled='true'])"
    )
    .evaluateAll((elements) =>
      elements.flatMap((element) => {
        if (!(element instanceof HTMLElement)) return []
        if (
          element.getAttribute('aria-label') ===
            'Open TanStack Router Devtools' ||
          element.getAttribute('aria-label') === 'Open Tanstack query devtools'
        ) {
          return []
        }
        let hitTarget = element
        if (
          element instanceof HTMLInputElement &&
          (element.type === 'checkbox' || element.type === 'radio')
        ) {
          const associatedLabel = element.labels?.item(0)
          if (associatedLabel instanceof HTMLElement) {
            hitTarget = associatedLabel
          }
        } else if (
          element.getAttribute('role') === 'checkbox' ||
          element.getAttribute('role') === 'radio'
        ) {
          const associatedLabel = element.closest('label')
          if (associatedLabel instanceof HTMLElement) {
            hitTarget = associatedLabel
          }
        }
        const rectangle = hitTarget.getBoundingClientRect()
        const style = getComputedStyle(hitTarget)
        if (
          rectangle.width === 0 ||
          rectangle.height === 0 ||
          style.display === 'none' ||
          style.visibility === 'hidden'
        ) {
          return []
        }
        if (rectangle.width >= 40 && rectangle.height >= 40) return []
        return [
          {
            height: Math.round(rectangle.height * 10) / 10,
            name:
              element.getAttribute('aria-label') ||
              element.textContent?.trim().slice(0, 40) ||
              element.tagName,
            tag: element.tagName,
            width: Math.round(rectangle.width * 10) / 10,
          },
        ]
      })
    )
  expect(undersized, `touch targets below 40px on ${route}`).toEqual([])
}

async function assertRouteSurface(
  page: Page,
  route: string,
  testInfo: TestInfo
) {
  await page.goto(route)
  await hideDeveloperOverlays(page)
  await expect(page.locator('main')).toBeVisible()
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth
      )
    )
    .toBe(true)
  await assertKeyboardFocus(page)
  await assertNoAccessibilityViolations(page, route)
  if (testInfo.project.name === 'chromium-mobile') {
    await assertTouchTargets(page, route)
  }
}

test('discovers the authenticated file-route surface', async ({
  page: _page,
}, testInfo) => {
  expect(authenticatedRoutes.length).toBeGreaterThanOrEqual(20)
  await testInfo.attach('audited-routes.json', {
    body: JSON.stringify(authenticatedRoutes, null, 2),
    contentType: 'application/json',
  })
})

for (const route of authenticatedRoutes) {
  test(`audits authenticated route ${route}`, async ({ page }, testInfo) => {
    await installAuthenticatedFailureBoundary(page)
    if (testInfo.project.name === 'chromium-mobile') {
      await page.setViewportSize({ height: 844, width: 375 })
    } else {
      await page.setViewportSize({ height: 900, width: 1440 })
    }
    await assertRouteSurface(page, route, testInfo)
  })
}

test('keeps the unauthenticated sign-in file route accessible and responsive', async ({
  page,
}, testInfo) => {
  if (testInfo.project.name === 'chromium-mobile') {
    await page.setViewportSize({ height: 844, width: 375 })
  } else {
    await page.setViewportSize({ height: 900, width: 1440 })
  }
  await assertRouteSurface(page, '/sign-in', testInfo)
})
