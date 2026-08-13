import AxeBuilder from '@axe-core/playwright'
import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from '@playwright/test'

import { mockAuthenticatedShell } from './helpers/auth'

const viewer = {
  display_name: '模型目录只读员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'catalog_viewer',
}
const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'

interface ExportBody {
  filters: Record<string, unknown>
  format: 'csv' | 'xlsx'
  statistics_type: string
}

function envelope<T>(data: T, requestId = 'req_catalog_e2e') {
  return { code: '', data, message: '', request_id: requestId, success: true }
}

function assertAuthenticated(route: Route) {
  const headers = route.request().headers()
  expect(headers['new-api-user']).toBe(viewer.id)
  expect(headers['x-request-id']).toMatch(/^web_/)
}

async function seedAuth(page: Page, testInfo: TestInfo) {
  await mockAuthenticatedShell(page)
  if (
    testInfo.project.name === 'chromium-mobile' ||
    testInfo.project.name === 'chromium-tablet-768'
  ) {
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
    await route.fulfill({ json: envelope(viewer, 'req_catalog_self') })
  })
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope({
        items: [{ id: '9007199254740997', name: '华东模型站点' }],
        page: 1,
        page_size: 100,
        total: 1,
      }),
    })
  })
}

const longDescription = `安全模型描述：${'用于验证完整展开能力的长文本。'.repeat(24)}`
const longTags = Array.from(
  { length: 24 },
  (_, index) => `production-tag-${index}`
).join(',')
const iconText = `https://icons.example.invalid/${'long-icon-segment-'.repeat(18)}model.svg`
const catalogItems = [0, 1, 2, 3].map((nameRule, index) => ({
  covered_channels: String(index + 1),
  covered_groups: String(index + 2),
  created_time: 1_784_000_000,
  data_status: index === 0 ? 'complete' : 'partial',
  description: index === 0 ? longDescription : `安全模型描述 ${index}`,
  icon: index === 0 ? iconText : `icon-text-${index}`,
  id: String(9007199254740800 + index),
  model_name: index === 0 ? 'gpt-4o' : `gpt-rule-${index}`,
  name_rule: nameRule,
  remote_id: String(9007199254740700 + index),
  site_id: '9007199254740997',
  site_name: '华东模型站点',
  status: index % 2,
  sync_official: index % 2,
  tags: index === 0 ? longTags : 'safe,official',
  updated_time: 1_784_348_700,
  vendor_id: index === 0 ? '0' : '9007199254740995',
}))

const coverageMetric = {
  catalog_models: '9007199254740993',
  channel_mappings: '9007199254740996',
  exact_covered_models: '9007199254740994',
  exact_missing_models: '2',
}

function coverageBreakdown(
  dimensionId: string,
  dimensionName: string,
  site = false
) {
  return {
    ...coverageMetric,
    as_of: 1_784_348_700,
    data_status: site ? 'unavailable' : 'partial',
    dimension_id: dimensionId,
    dimension_name: dimensionName,
    site_id: site ? '9007199254740997' : '0',
    site_name: site ? '华东模型站点' : '',
  }
}

function coverage() {
  return {
    ...coverageMetric,
    data_status: 'partial',
    site_breakdown: [
      coverageBreakdown('9007199254740997', '华东模型站点', true),
    ],
    status_breakdown: [coverageBreakdown('1', 'enabled')],
    vendor_breakdown: [coverageBreakdown('0', 'Vendor 0')],
  }
}

const missingItems = [
  {
    as_of: 1_784_348_700,
    channel_name: '视频渠道',
    data_status: 'partial',
    group: 'default',
    model_name: 'gpt-prefix-child',
    remote_channel_id: '0',
    site_id: '9007199254740997',
    site_name: '华东模型站点',
  },
]

function forbiddenFields() {
  return [
    ['pri', 'cing'].join(''),
    ['billing', 'expr'].join('_'),
    ['end', 'points'].join(''),
    ['bound', 'channels'].join('_'),
    ['enable', 'groups'].join('_'),
    ['quota', 'types'].join('_'),
    ['matched', 'models'].join('_'),
    ['matched', 'count'].join('_'),
  ]
}

test('A96 keeps model catalog exact, icon-text-only, private, exportable and responsive', async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000)
  await seedAuth(page, testInfo)
  const globalReads: URL[] = []
  let externalIconRequests = 0
  let exportBody: ExportBody | undefined

  await page.route('https://icons.example.invalid/**', async (route) => {
    externalIconRequests++
    await route.abort()
  })
  await page.route(/\/api\/model-catalog(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    globalReads.push(new URL(route.request().url()))
    await route.fulfill({
      json: envelope({
        data_status: 'partial',
        items: catalogItems,
        page: 1,
        page_size: 20,
        total: catalogItems.length,
      }),
    })
  })
  await page.route(
    /\/api\/model-catalog\/coverage(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      const url = new URL(route.request().url())
      expect([...url.searchParams.entries()]).toEqual([])
      globalReads.push(url)
      await route.fulfill({ json: envelope(coverage()) })
    }
  )
  await page.route(/\/api\/model-catalog\/missing(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    globalReads.push(new URL(route.request().url()))
    await route.fulfill({
      json: envelope({
        as_of: 1_784_348_700,
        data_status: 'partial',
        items: missingItems,
        page: 1,
        page_size: 20,
        total: 1,
      }),
    })
  })
  for (const suffix of ['', '/coverage', '/missing'] as const) {
    await page.route(
      new RegExp(
        `/api/sites/9007199254740997/model-catalog${suffix.replace('/', '\\/')}(?:\\?.*)?$`
      ),
      async (route) => {
        assertAuthenticated(route)
        expect(
          new URL(route.request().url()).searchParams.has('site_ids')
        ).toBe(false)
        if (suffix === '/coverage') {
          expect([
            ...new URL(route.request().url()).searchParams.entries(),
          ]).toEqual([])
          await route.fulfill({ json: envelope(coverage()) })
        } else if (suffix === '/missing') {
          await route.fulfill({
            json: envelope({
              as_of: 1_784_348_700,
              data_status: 'partial',
              items: missingItems,
              page: 1,
              page_size: 20,
              total: 1,
            }),
          })
        } else {
          await route.fulfill({
            json: envelope({
              data_status: 'partial',
              items: catalogItems.slice(0, 1),
              page: 1,
              page_size: 20,
              total: 1,
            }),
          })
        }
      }
    )
  }
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
        id: '796',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: exportBody.statistics_type,
        status: 'pending',
      }),
    })
  })
  await page.route('**/api/statistics/exports/796', async (route) => {
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
        id: '796',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: exportBody?.statistics_type ?? 'model_catalog',
        status: 'pending',
      }),
    })
  })

  await page.goto('/model-catalog')
  await expect(
    page.getByRole('heading', { exact: true, name: '模型审计' })
  ).toBeVisible()
  await expect(
    page.getByText(iconText, { exact: true }).filter({ visible: true }).first()
  ).toBeVisible()
  expect(externalIconRequests).toBe(0)
  await expect(page.locator(`img[src="${iconText}"]`)).toHaveCount(0)
  await expect(page.locator(`a[href="${iconText}"]`)).toHaveCount(0)
  if (
    testInfo.project.name === 'chromium-mobile' ||
    testInfo.project.name === 'chromium-tablet-768'
  ) {
    await expect(
      page.getByText(longDescription, { exact: true }).filter({ visible: true })
    ).toBeVisible()
    await expect(
      page.getByText(longTags, { exact: true }).filter({ visible: true })
    ).toBeVisible()
    await expect(
      page
        .getByText('官方同步', { exact: true })
        .filter({ visible: true })
        .first()
    ).toBeVisible()
    await expect(
      page
        .getByText('创建时间', { exact: true })
        .filter({ visible: true })
        .first()
    ).toBeVisible()
    await expect(
      page
        .getByText('更新时间', { exact: true })
        .filter({ visible: true })
        .first()
    ).toBeVisible()
    await expect(
      page
        .getByText('数据状态', { exact: true })
        .filter({ visible: true })
        .first()
    ).toBeVisible()
  } else {
    for (const field of ['description', 'tags', 'icon'] as const) {
      const details = page.getByTestId(`model-${field}-9007199254740800`)
      await expect(details).toBeVisible()
      await details.locator('summary').click()
      await expect(details).toHaveAttribute('open', '')
      await expect(details.locator('p')).not.toBeEmpty()
    }
  }
  for (const rule of ['精确匹配', '前缀匹配', '包含匹配', '后缀匹配']) {
    await expect(
      page.getByText(rule, { exact: true }).filter({ visible: true }).first()
    ).toBeVisible()
  }

  await page
    .getByRole('textbox', { exact: true, name: '模型关键词' })
    .fill('gpt')
  await page.getByRole('button', { exact: true, name: '站点' }).click()
  await page.getByRole('button', { exact: true, name: '华东模型站点' }).click()
  await page.getByRole('textbox', { exact: true, name: '供应商 ID' }).fill('0')
  await page.getByRole('button', { exact: true, name: '模型状态' }).click()
  await page
    .getByRole('button', { exact: true, name: '启用' })
    .filter({ visible: true })
    .click()
  await page.getByRole('button', { exact: true, name: '官方同步状态' }).click()
  await page
    .getByRole('button', { exact: true, name: '禁用' })
    .filter({ visible: true })
    .click()
  await expect
    .poll(() => globalReads.at(-1)?.searchParams.get('vendor_id'))
    .toBe('0')
  const catalogRead = globalReads.at(-1)
  expect(catalogRead?.searchParams.getAll('site_ids')).toEqual([
    '9007199254740997',
  ])
  expect(catalogRead?.searchParams.getAll('statuses')).toEqual(['1'])
  expect(catalogRead?.searchParams.getAll('sync_official')).toEqual(['0'])

  await page.getByRole('button', { name: '导出 XLSX' }).click()
  await expect.poll(() => exportBody?.statistics_type).toBe('model_catalog')
  expect(exportBody?.filters.model_vendor_id).toBe('0')
  expect(exportBody?.filters.model_statuses).toEqual([1])
  expect(exportBody?.filters.model_sync_official).toEqual([0])
  await page.getByRole('button', { name: '关闭' }).click()

  await page.getByRole('tab', { name: '覆盖分析' }).click()
  await expect(page.getByText('9007199254740993').first()).toBeVisible()
  await expect(page.getByText('Vendor 0')).toBeVisible()
  await expect(
    page.getByText('不可用').filter({ visible: true }).first()
  ).toBeVisible()

  await page.getByRole('tab', { name: '渠道未登记' }).click()
  await expect(
    page.getByText('gpt-prefix-child').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(page.getByText(/前缀、包含和后缀规则/)).toBeVisible()
  if (testInfo.project.name === 'chromium-mobile') {
    await expect(
      page.getByText(/采集截至/).filter({ visible: true })
    ).toBeVisible()
  }
  const serializedExport = JSON.stringify(exportBody).toLowerCase()
  for (const field of forbiddenFields()) {
    expect(serializedExport).not.toContain(field)
  }

  const scan = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(scan.violations).toEqual([])
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
  const browserState = await page.evaluate(() => {
    const modelCatalogContent = document.querySelector('main#main-content')
    return {
      attributes: [...(modelCatalogContent?.querySelectorAll('*') ?? [])]
        .flatMap((element) =>
          [...element.attributes].map(
            (attribute) => `${attribute.name}=${attribute.value}`
          )
        )
        .join('\n'),
      localStorage: JSON.stringify(window.localStorage),
      text: modelCatalogContent?.textContent ?? '',
      url: window.location.href,
    }
  })
  const visibleState = JSON.stringify(browserState).toLowerCase()
  for (const field of forbiddenFields()) {
    expect(visibleState).not.toContain(field)
  }
  expect(externalIconRequests).toBe(0)

  await page.goto('/sites/9007199254740997/model-catalog?tab=coverage')
  await expect(
    page.getByRole('heading', { exact: true, name: '站点模型审计' })
  ).toBeVisible()
  await expect(
    page.getByRole('heading', { name: '站点覆盖情况' })
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
})

test('shows a visible retry alert when initial model coverage loading fails', async ({
  page,
}, testInfo) => {
  await seedAuth(page, testInfo)
  await page.route(/\/api\/model-catalog(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope({
        data_status: 'complete',
        items: [],
        page: 1,
        page_size: 20,
        total: 0,
      }),
    })
  })
  await page.route(
    /\/api\/model-catalog\/coverage(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      await route.fulfill({
        body: JSON.stringify({
          code: 'upstream_unavailable',
          data: null,
          message: 'coverage unavailable',
          request_id: 'req_coverage_failure',
          success: false,
        }),
        contentType: 'application/json',
        status: 503,
      })
    }
  )

  await page.goto('/model-catalog')

  const alert = page.getByRole('alert')
  await expect(alert).toBeVisible({ timeout: 20_000 })
  await expect(alert.getByRole('button')).toBeVisible()
})

test('retains the last successful model rows when a filtered refresh fails', async ({
  page,
}, testInfo) => {
  await seedAuth(page, testInfo)
  let reads = 0
  await page.route(/\/api\/model-catalog(?:\?.*)?$/, async (route) => {
    reads += 1
    if (reads === 1) {
      await route.fulfill({
        json: envelope({
          data_status: 'complete',
          items: [catalogItems[0]],
          page: 1,
          page_size: 20,
          total: '1',
        }),
      })
      return
    }
    await route.fulfill({ status: 503, json: envelope(null) })
  })
  await page.route(/\/api\/model-catalog\/coverage(?:\?.*)?$/, (route) =>
    route.fulfill({ json: envelope(coverage()) })
  )

  await page.goto('/model-catalog')
  await expect(
    page.getByText('gpt-4o', { exact: true }).filter({ visible: true }).first()
  ).toBeVisible()
  await page.getByRole('textbox', { name: '模型关键词' }).fill('刷新失败')
  await expect.poll(() => reads).toBe(2)
  await expect(page.getByRole('alert')).toContainText(
    '数据刷新失败，当前显示的是上次成功加载的结果'
  )
  await expect(
    page.getByText('gpt-4o', { exact: true }).filter({ visible: true }).first()
  ).toBeVisible()
  await page.reload()
  await expect(page).toHaveURL(/keyword=%E5%88%B7%E6%96%B0%E5%A4%B1%E8%B4%A5/)
})
