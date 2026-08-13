import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import zhCN from '@/i18n/locales/zh-CN.json'

const pagePath = new URL('./components/customers-page.tsx', import.meta.url)
const cardPath = new URL('./components/customer-ui.tsx', import.meta.url)
const dialogsPath = new URL(
  './components/customer-dialogs.tsx',
  import.meta.url
)
const filtersPath = new URL(
  './components/customer-filters.tsx',
  import.meta.url
)
const detailPath = new URL(
  './components/customer-detail-page.tsx',
  import.meta.url
)
const emptyStatePath = new URL('../../components/ui/empty.tsx', import.meta.url)

describe('customer management production states', () => {
  test('distinguishes filtered results from an empty customer inventory', async () => {
    const [source, filtersSource] = await Promise.all([
      readFile(pagePath, 'utf8'),
      readFile(filtersPath, 'utf8'),
    ])

    expect(source).toContain("'customers.noResults'")
    expect(source).toContain("'customers.noResultsDescription'")
    expect(zhCN['customers.noResults']).toBe('未找到匹配的客户')
    expect(zhCN['customers.noResultsDescription']).toBe(
      '请调整搜索条件或客户状态后重试。'
    )
    expect(filtersSource).toContain('<MultiFacetedFilter')
    expect(filtersSource).toContain('values={draft.status}')
    expect(source).not.toContain("fetching && 'pointer-events-none")
  })

  test('keeps customer mutations and operational timestamps consistent', async () => {
    const [pageSource, cardSource, detailSource] = await Promise.all([
      readFile(pagePath, 'utf8'),
      readFile(cardPath, 'utf8'),
      readFile(detailPath, 'utf8'),
    ])

    expect(pageSource).toContain('queryKey: statisticsKeys.all')
    expect(detailSource).toContain('queryKey: statisticsKeys.all')
    expect(detailSource).toContain('entityDetailFailure(')
    expect(detailSource).toContain("failure.kind === 'invalid-id'")
    expect(detailSource).toContain(
      'onRetry={failure.retryable ? retry : undefined}'
    )
    expect(pageSource).not.toContain("header: t('common.updatedAt')")
    expect(cardSource).not.toContain("t('common.updatedAt')")
    expect(cardSource).toContain(
      'formatDecimalDisplayValue(customer.contract_amount)'
    )
    expect(cardSource).toContain(
      'formatDecimalDisplayValue(customer.payment_amount)'
    )
    expect(pageSource).toContain(
      'formatDecimalDisplayValue(row.original.contract_amount)'
    )
    expect(detailSource).toContain(
      'formatDecimalDisplayValue(customer.contract_amount)'
    )
  })

  test('matches the site card metric hierarchy and grouped list layout', async () => {
    const [pageSource, cardSource] = await Promise.all([
      readFile(pagePath, 'utf8'),
      readFile(cardPath, 'utf8'),
    ])

    expect(cardSource).toContain('text-base leading-tight font-semibold')
    expect(cardSource).toContain('customer.today.token_used')
    expect(cardSource).toContain('customer.today.avg_rpm')
    expect(cardSource).toContain('customer.today.avg_tpm')
    expect(cardSource).toContain("to='/customers/$customerId/stats'")
    expect(cardSource).toContain("to='/customers/$customerId'")
    expect(pageSource).toContain("t('customer.list.identity')")
    expect(pageSource).toContain("t('customer.list.business')")
    expect(pageSource).toContain("t('customer.list.todayUsage')")
    expect(pageSource).toContain("t('site.dashboard.todayQuota')")
    expect(pageSource).toContain("t('site.dashboard.todayTokens')")
    expect(pageSource).toContain("t('site.dashboard.todayCount')")
    expect(cardSource).toContain("t('site.dashboard.todayQuota')")
    expect(cardSource).not.toContain("t('site.dashboard.totalQuota')")
    expect(pageSource).toContain('useLastValidPage({')
    expect(pageSource).toContain('const listStale =')
  })

  test('surfaces status errors and locks the drawer while saving', async () => {
    const source = await readFile(dialogsPath, 'utf8')

    expect(source).toContain("errors.status?.type === 'server'")
    expect(source).toContain('!open && !pending && requestClose()')
    expect(source).toContain('disabled={pending}')
  })

  test('gives customer empty and error states semantic headings', async () => {
    const source = await readFile(emptyStatePath, 'utf8')

    expect(source).toContain("React.ComponentProps<'h2'>")
    expect(source).toContain('<h2')
    expect(source).not.toContain('<h3')
  })
})
