import { expect, test, type Page } from '@playwright/test'

import { mockAuthenticatedShell } from './helpers/auth'

const admin = {
  display_name: '路由审查管理员',
  id: '1',
  must_change_password: false,
  role: 'admin' as const,
  status: 1 as const,
  username: 'admin',
}

function envelope(data: unknown) {
  return {
    code: '',
    data,
    message: '',
    request_id: 'req_route_state',
    success: true,
  }
}

function errorEnvelope(code: string) {
  return {
    code,
    data: null,
    message: code,
    request_id: 'req_route_error',
    success: false,
  }
}

async function seedAuth(page: Page) {
  await mockAuthenticatedShell(page)
  await page.addInitScript((user) => {
    localStorage.setItem('pilot-auth-user', JSON.stringify(user))
    localStorage.setItem('uid', user.id)
  }, admin)
  await page.route('**/api/user/self', (route) =>
    route.fulfill({ json: envelope(admin) })
  )
}

test('unknown routes expose two recovery paths', async ({ page }) => {
  await seedAuth(page)
  await page.goto('/unknown-production-route')
  await expect(page.getByRole('heading', { name: '页面不存在' })).toBeVisible()
  await expect(page.getByRole('button', { name: '返回上一页' })).toBeVisible()
  await expect(page.getByRole('button', { name: '返回工作台' })).toBeVisible()
})

test('entity 404 is terminal while a server failure remains retryable', async ({
  page,
}) => {
  await seedAuth(page)
  await page.route('**/api/sites/999/instances', (route) =>
    route.fulfill({ json: envelope([]) })
  )
  await page.route('**/api/sites/999/performance**', (route) =>
    route.fulfill({ json: envelope({ models: [] }) })
  )
  let failureStatus = 404
  await page.route(/\/api\/sites\/999(?:\?.*)?$/, async (route) => {
    if (failureStatus === 404) {
      await route.fulfill({ json: errorEnvelope('NOT_FOUND'), status: 404 })
      return
    }
    await route.fulfill({ json: errorEnvelope('INTERNAL_ERROR'), status: 503 })
  })

  await page.goto('/sites/999')
  await expect(page.getByText('未找到请求的对象')).toBeVisible()
  await expect(page.getByRole('button', { name: '重试' })).toHaveCount(0)

  failureStatus = 503
  await page.reload()
  await expect(page.getByRole('button', { name: '重试' })).toBeVisible()
})

test('representative route states fit each production viewport', async ({
  page,
}) => {
  await seedAuth(page)
  for (const width of [390, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 900 })
    await page.goto('/unknown-production-route')
    await expect
      .poll(() =>
        page.evaluate(
          () =>
            document.documentElement.scrollWidth <=
            document.documentElement.clientWidth
        )
      )
      .toBe(true)
  }
})
