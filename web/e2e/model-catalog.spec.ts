import AxeBuilder from '@axe-core/playwright'
import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from '@playwright/test'

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
    await route.fulfill({ json: envelope(viewer, 'req_catalog_self') })
  })
}

const iconText = 'https://icons.example.invalid/model.svg'
const catalogItems = [0, 1, 2, 3].map((nameRule, index) => ({
  covered_channels: String(index + 1),
  covered_groups: String(index + 2),
  created_time: 1_784_000_000,
  data_status: index === 0 ? 'complete' : 'partial',
  description: `安全模型描述 ${index}`,
  icon: index === 0 ? iconText : `icon-text-${index}`,
  id: String(9007199254740800 + index),
  model_name: index === 0 ? 'gpt-4o' : `gpt-rule-${index}`,
  name_rule: nameRule,
  remote_id: String(9007199254740700 + index),
  site_id: '9007199254740997',
  site_name: '华东模型站点',
  status: index % 2,
  sync_official: index % 2,
  tags: 'safe,official',
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
      globalReads.push(new URL(route.request().url()))
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
    page.getByRole('heading', { exact: true, name: '模型目录' })
  ).toBeVisible()
  await expect(
    page.getByText(iconText, { exact: true }).filter({ visible: true }).first()
  ).toBeVisible()
  expect(externalIconRequests).toBe(0)
  await expect(page.locator(`img[src="${iconText}"]`)).toHaveCount(0)
  await expect(page.locator(`a[href="${iconText}"]`)).toHaveCount(0)
  for (const rule of [
    'Exact 精确',
    'Prefix 前缀',
    'Contains 包含',
    'Suffix 后缀',
  ]) {
    await expect(
      page.getByText(rule, { exact: true }).filter({ visible: true }).first()
    ).toBeVisible()
  }

  await page
    .getByRole('textbox', { exact: true, name: '模型关键词' })
    .fill('gpt')
  await page
    .getByRole('textbox', { exact: true, name: '站点 ID' })
    .fill('9007199254740997')
  await page.getByRole('textbox', { exact: true, name: 'Vendor ID' }).fill('0')
  await page
    .getByRole('group', { name: '模型状态' })
    .getByRole('button', { name: '启用' })
    .click()
  await page
    .getByRole('group', { name: '官方同步状态' })
    .getByRole('button', { name: '禁用' })
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

  await page.getByRole('tab', { name: 'Coverage' }).click()
  await expect(page.getByText('9007199254740993').first()).toBeVisible()
  await expect(page.getByText('Vendor 0')).toBeVisible()
  await expect(
    page.getByText('不可用').filter({ visible: true }).first()
  ).toBeVisible()

  await page.getByRole('tab', { name: 'Missing' }).click()
  await expect(
    page.getByText('gpt-prefix-child').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(page.getByText(/Prefix、Contains、Suffix/)).toBeVisible()
  await page.getByRole('button', { name: '导出 XLSX' }).click()
  await expect.poll(() => exportBody?.statistics_type).toBe('model_catalog')
  expect(exportBody?.filters.model_vendor_id).toBe('0')
  expect(exportBody?.filters.model_statuses).toEqual([1])
  expect(exportBody?.filters.model_sync_official).toEqual([0])
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
    page.getByRole('heading', { exact: true, name: '站点模型目录' })
  ).toBeVisible()
  await expect(page.getByText('站点覆盖拆分')).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
})
