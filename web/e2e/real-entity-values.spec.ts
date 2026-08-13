import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/sign-in')
  await page.getByLabel('用户名').fill('admin')
  await page.getByRole('textbox', { name: '密码' }).fill('change-me')
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/dashboard$/)
}

async function expectValues(
  page: import('@playwright/test').Page,
  path: string,
  values: readonly string[]
) {
  await page.goto(path)
  for (const value of values) {
    await expect(
      page.getByText(value, { exact: true }).filter({ visible: true }).first()
    ).toBeVisible()
  }
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
}

test.beforeEach(async ({ page }) => login(page))

test('real site list and detail match the DB/API values', async ({ page }) => {
  await expectValues(page, '/sites', ['本地站点', '1/1'])
  await expectValues(page, '/sites/1', ['本地站点', '500,000', '6.82'])
})

test('real customer list and detail match the DB/API values', async ({
  page,
}) => {
  await expectValues(page, '/customers', ['测试客户', '1,000,000', '1,000'])
  await expectValues(page, '/customers/1', ['测试客户', '1,000,000', '1,000'])
})

test('real account list and detail match the DB/API values', async ({
  page,
}) => {
  await expectValues(page, '/accounts', ['test', '493万', '7万'])
  await expect(
    page.locator('[title="4929887"]').filter({ visible: true })
  ).toBeVisible()
  await expect(
    page.locator('[title="70113"]').filter({ visible: true })
  ).toBeVisible()
  await expectValues(page, '/accounts/1', [
    'test',
    '4,929,887',
    '70,113',
    '共 812 个窗口，已完成 807 个，失败 5 个',
  ])
})

test('real site system tasks match the authenticated API values', async ({
  page,
}) => {
  const listResponsePromise = page.waitForResponse(
    (response) =>
      response.url().includes('/api/sites/1/system-tasks?') &&
      !response.url().includes('/statistics') &&
      response.request().method() === 'GET'
  )
  const statisticsResponsePromise = page.waitForResponse(
    (response) =>
      response.url().includes('/api/sites/1/system-tasks/statistics') &&
      response.request().method() === 'GET'
  )

  await page.goto('/sites/1/system-tasks')
  const [listResponse, statisticsResponse] = await Promise.all([
    listResponsePromise,
    statisticsResponsePromise,
  ])
  expect(listResponse.ok()).toBe(true)
  expect(statisticsResponse.ok()).toBe(true)

  const listEnvelope = (await listResponse.json()) as {
    data: {
      items: Array<{ remote_id: string; task_id: string }>
      total: string
    }
  }
  const statisticsEnvelope = (await statisticsResponse.json()) as {
    data: { summary: { total: string } }
  }
  const total = listEnvelope.data.total
  expect(statisticsEnvelope.data.summary.total).toBe(total)
  await expect(
    page
      .getByText(Number(total).toLocaleString('en-US'), { exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()

  const latest = listEnvelope.data.items[0]
  expect(latest).toBeDefined()
  if (latest) {
    await expect(
      page.getByText(latest.task_id).filter({ visible: true }).first()
    ).toBeVisible()
    await expect(
      page
        .getByText(latest.remote_id, { exact: true })
        .filter({ visible: true })
    ).toBeVisible()
  }
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
})
