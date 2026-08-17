import { expect, test, type Page } from '@playwright/test'

const destinations = [
  {
    heading: '客户管理',
    label: '客户管理',
    path: '/customers',
    url: /\/customers(?:\?|$)/,
  },
  {
    heading: '系统任务',
    label: '系统任务',
    path: '/system-tasks',
    url: /\/system-tasks(?:\?|$)/,
  },
  { heading: '系统设置', label: '系统设置', path: '/settings/system' },
] as const

const sources = [
  { heading: '模型审计', name: 'model catalog', path: '/model-catalog' },
  {
    heading: '定价与分组',
    name: 'pricing and groups',
    path: '/pricing-groups',
  },
] as const

test.describe.configure({ mode: 'serial' })

async function login(page: Page) {
  await page.goto('/sign-in')
  await page.getByLabel('用户名').fill('admin')
  await page.getByRole('textbox', { name: '密码' }).fill('change-me')
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/dashboard$/)
}

function collectRuntimeErrors(page: Page) {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text())
  })
  return errors
}

async function expectGlobalNavigationEntry(
  page: Page,
  label: string,
  path: string
) {
  const navigation = page.getByRole('navigation', { name: '主导航' })
  const link = navigation.getByRole('link', { name: label, exact: true })
  if (!(await link.isVisible())) {
    const mobileNavigation = page.getByRole('button', {
      name: '打开导航',
      exact: true,
    })
    if (await mobileNavigation.isVisible()) await mobileNavigation.click()
  }
  await expect(link).toBeVisible()
  await expect(link).toHaveAttribute('href', path)
  return link
}

test.beforeEach(async ({ page }) => login(page))

for (const source of sources) {
  for (const destination of destinations) {
    test(`${source.name} navigates independently to ${destination.path}`, async ({
      page,
    }) => {
      const errors = collectRuntimeErrors(page)
      await page.goto(source.path)
      await expect(page.getByRole('heading', { level: 1 })).toHaveText(
        source.heading
      )

      const link = await expectGlobalNavigationEntry(
        page,
        destination.label,
        destination.path
      )
      await link.click()

      await page.waitForTimeout(100)
      expect(errors).toEqual([])

      await expect(page).toHaveURL(
        'url' in destination ? destination.url : destination.path
      )
      await expect(page.getByRole('heading', { level: 1 })).toHaveText(
        destination.heading
      )
    })
  }
}

test('brand home link independently returns from model catalog to dashboard', async ({
  page,
}) => {
  const errors = collectRuntimeErrors(page)
  await page.goto('/model-catalog')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('模型审计')

  const homeLink = page.getByRole('link', { name: '返回首页', exact: true })
  await expect(homeLink).toHaveAttribute('href', '/dashboard')
  await homeLink.click()

  await expect(page).toHaveURL('/dashboard')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('运营概览')
  expect(errors).toEqual([])
})

test('a pasted URL with a Chinese comma is an explicit unknown route', async ({
  page,
}) => {
  await page.goto('/model-catalog，')

  await expect(page).toHaveURL(/\/model-catalog(?:%EF%BC%8C|，)$/)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('页面不存在')
})
