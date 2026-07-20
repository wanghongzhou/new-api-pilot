import AxeBuilder from '@axe-core/playwright'
import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from '@playwright/test'

const viewer = {
  display_name: '财务运营只读员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'finance_viewer',
}
const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'

interface ExportBody {
  filters: Record<string, unknown>
  format: 'csv' | 'xlsx'
  statistics_type: string
}

function envelope<T>(data: T, requestId = 'req_finance_e2e') {
  return { code: '', data, message: '', request_id: requestId, success: true }
}

function assertAuthenticated(route: Route) {
  const headers = route.request().headers()
  expect(headers['new-api-user']).toBe(viewer.id)
  expect(headers['x-request-id']).toMatch(/^web_/)
}

async function seedAuth(page: Page, testInfo: TestInfo) {
  if (testInfo.project.name === 'chromium-mobile') {
    await page.setViewportSize({ height: 812, width: 375 })
  }
  await page.addInitScript(
    ({ authKey, authUser, uidKey }) => {
      window.localStorage.setItem(authKey, JSON.stringify(authUser))
      window.localStorage.setItem(uidKey, authUser.id)
    },
    { authKey: authStorageKey, authUser: viewer, uidKey: uidStorageKey }
  )
  await page.route('**/api/user/self', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({ json: envelope(viewer, 'req_finance_self') })
  })
}

const topup = {
  amount: '9007199254740993',
  complete_time: 1_784_348_700,
  create_time: 1_784_348_000,
  first_seen_at: 1_784_348_100,
  id: '9007199254740993',
  last_seen_at: 1_784_348_700,
  missing_count: 0,
  money: '123456789012345678.1234567890',
  payment_method: 'card',
  payment_provider: 'provider-a',
  remote_id: '9007199254740995',
  remote_state: 'normal',
  remote_user_id: '0',
  site_id: '9007199254740997',
  site_name: '华东充值站点',
  status: 'success',
}

const redemption = {
  created_time: 1_784_262_400,
  derived_status: 'expired',
  expired_time: 1_784_300_000,
  first_seen_at: 1_784_262_500,
  id: '9007199254740994',
  last_seen_at: null,
  missing_count: 2,
  name: '夏季活动批次',
  quota: '9223372036854775807',
  redeemed_time: 0,
  remote_id: '9007199254740996',
  remote_state: 'missing',
  remote_user_id: '0',
  site_id: '9007199254740997',
  site_name: '华东充值站点',
  status: 1,
  used_user_id: '0',
}

const breakdownBase = {
  as_of: 1_784_348_700,
  count: '1',
  data_status: 'complete',
  missing_count: '0',
  site_id: '9007199254740997',
  site_name: '华东充值站点',
}

function topupStatistics() {
  return {
    data_status: 'complete',
    provider_breakdown: [
      {
        ...breakdownBase,
        amount: '9007199254740993',
        dimension_id: 'provider-a',
        dimension_name: 'provider-a',
        money: '123456789012345678.1234567890',
      },
    ],
    site_breakdown: [
      {
        ...breakdownBase,
        amount: '9007199254740993',
        dimension_id: '9007199254740997',
        dimension_name: '华东充值站点',
        money: '123456789012345678.1234567890',
      },
    ],
    status_breakdown: [
      {
        ...breakdownBase,
        dimension_id: 'success',
        dimension_name: 'success',
        site_id: '',
        site_name: '',
      },
    ],
    summary: { count: '1', missing_count: '0' },
  }
}

function redemptionStatistics() {
  return {
    data_status: 'partial',
    site_breakdown: [
      {
        ...breakdownBase,
        data_status: 'partial',
        dimension_id: '9007199254740997',
        dimension_name: '华东充值站点',
        missing_count: '1',
        quota: '9223372036854775807',
      },
    ],
    status_breakdown: [
      {
        ...breakdownBase,
        data_status: 'partial',
        dimension_id: 'expired',
        dimension_name: 'expired',
        missing_count: '1',
        quota: '9223372036854775807',
        site_id: '',
        site_name: '',
      },
    ],
    summary: {
      count: '1',
      missing_count: '1',
      quota: '9223372036854775807',
    },
  }
}

test('keeps finance operations exact, non-reconciling, secret-free, exportable and responsive', async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000)
  await seedAuth(page, testInfo)
  const topupReads: URL[] = []
  const redemptionReads: URL[] = []
  let exportBody: ExportBody | undefined

  await page.route(/\/api\/topups(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    topupReads.push(new URL(route.request().url()))
    await route.fulfill({
      json: envelope({
        as_of: 1_784_348_700,
        data_status: 'complete',
        items: [topup],
        page: 1,
        page_size: 20,
        total: 1,
      }),
    })
  })
  await page.route(/\/api\/topups\/statistics(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    topupReads.push(new URL(route.request().url()))
    await route.fulfill({ json: envelope(topupStatistics()) })
  })
  await page.route(/\/api\/redemptions(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    redemptionReads.push(new URL(route.request().url()))
    await route.fulfill({
      json: envelope({
        as_of: 1_784_348_700,
        data_status: 'partial',
        items: [redemption],
        page: 1,
        page_size: 20,
        total: 1,
      }),
    })
  })
  await page.route(
    /\/api\/redemptions\/statistics(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      redemptionReads.push(new URL(route.request().url()))
      await route.fulfill({ json: envelope(redemptionStatistics()) })
    }
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/redemptions(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      expect(new URL(route.request().url()).searchParams.has('site_ids')).toBe(
        false
      )
      await route.fulfill({
        json: envelope({
          as_of: 1_784_348_700,
          data_status: 'partial',
          items: [redemption],
          page: 1,
          page_size: 20,
          total: 1,
        }),
      })
    }
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/redemptions\/statistics(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      await route.fulfill({ json: envelope(redemptionStatistics()) })
    }
  )
  await page.route('**/api/statistics/export', async (route) => {
    assertAuthenticated(route)
    exportBody = route.request().postDataJSON() as ExportBody
    await route.fulfill({
      json: envelope({
        created_at: 1_784_348_800,
        data_snapshot_at: null,
        deduplicated: false,
        error: null,
        expires_at: null,
        file_name: '',
        file_size: '0',
        filters: exportBody.filters,
        finished_at: null,
        format: exportBody.format,
        id: '793',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: exportBody.statistics_type,
        status: 'pending',
      }),
    })
  })
  await page.route('**/api/statistics/exports/793', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope({
        created_at: 1_784_348_800,
        data_snapshot_at: null,
        deduplicated: false,
        error: null,
        expires_at: null,
        file_name: '',
        file_size: '0',
        filters: exportBody?.filters ?? {},
        finished_at: null,
        format: exportBody?.format ?? 'csv',
        id: '793',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: exportBody?.statistics_type ?? 'topup_inventory',
        status: 'pending',
      }),
    })
  })

  await page.goto('/financial-operations')
  await expect(
    page.getByRole('heading', { exact: true, name: '财务运营' })
  ).toBeVisible()
  await expect(
    page.getByText('充值金额是 provider 名义值，不是统一货币')
  ).toBeVisible()
  await expect(
    page
      .getByText('123456789012345678.1234567890', { exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect(page.getByText(/不能称为钱包对账/)).toBeVisible()

  await page
    .getByRole('textbox', { exact: true, name: '站点 ID' })
    .fill('9007199254740997')
  await page
    .getByRole('textbox', { exact: true, name: '远端用户 ID' })
    .fill('0')
  await page
    .getByRole('textbox', { exact: true, name: '支付 Provider' })
    .fill('provider-a')
  await page
    .getByRole('textbox', { exact: true, name: '支付方式' })
    .fill('card')
  await page.getByRole('button', { name: '本轮缺失' }).click()
  await expect
    .poll(() => topupReads.at(-1)?.searchParams.getAll('site_ids'))
    .toEqual(['9007199254740997'])
  await expect
    .poll(() => topupReads.at(-1)?.searchParams.get('remote_user_id'))
    .toBe('0')
  await expect
    .poll(() => topupReads.at(-1)?.searchParams.getAll('providers'))
    .toEqual(['provider-a'])

  await page.getByRole('button', { name: '导出 XLSX' }).click()
  await expect.poll(() => exportBody?.statistics_type).toBe('topup_inventory')
  expect(exportBody?.filters.finance_providers).toEqual(['provider-a'])
  const topupExport = JSON.stringify(exportBody).toLowerCase()
  expect(topupExport).not.toContain('payment_reference')
  expect(topupExport).not.toContain('secret')

  await page.goto('/financial-operations?tab=redemptions')
  await expect(page.getByRole('tab', { name: '兑换码' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  await expect(
    page.getByText('已过期').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page
      .getByText(/9223372036854775807/)
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await page
    .getByRole('textbox', { exact: true, name: '兑换批次名称' })
    .fill('夏季活动')
  await expect
    .poll(() => redemptionReads.at(-1)?.searchParams.get('keyword'))
    .toBe('夏季活动')
  await page.getByRole('button', { name: '导出 CSV' }).click()
  await expect
    .poll(() => exportBody?.statistics_type)
    .toBe('redemption_inventory')

  const accessibilityScan = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(accessibilityScan.violations).toEqual([])
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
  const browserState = await page.evaluate(() => ({
    attributes: [...document.querySelectorAll('*')]
      .flatMap((element) =>
        [...element.attributes].map(
          (attribute) => `${attribute.name}=${attribute.value}`
        )
      )
      .join('\n'),
    localStorage: JSON.stringify(window.localStorage),
    url: window.location.href,
  }))
  const visibleState = JSON.stringify(browserState).toLowerCase()
  expect(visibleState).not.toContain('payment_reference')
  expect(visibleState).not.toContain('secret')

  await page.goto(
    '/sites/9007199254740997/financial-operations?tab=redemptions'
  )
  await expect(
    page.getByRole('heading', { name: '站点财务运营' })
  ).toBeVisible()
  await expect(
    page.getByText('夏季活动批次').filter({ visible: true }).first()
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
})
