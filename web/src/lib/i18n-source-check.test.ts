import { describe, expect, test } from 'bun:test'

import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { MESSAGE_CODES } from '@/lib/message-codes'

import {
  findBootstrapHtmlViolations,
  findDuplicateJsonKeys,
  findHardcodedVisibleText,
  findMissingTranslationKeys,
  findTranslationKeyUsage,
  findUnknownDynamicRegistries,
  parseDynamicI18nRegistries,
} from './i18n-source-check'

describe('i18n source checks', () => {
  test('finds duplicate JSON keys before JSON parsing collapses them', () => {
    expect(
      findDuplicateJsonKeys('{"site.title":"站点","site.title":"重复"}')
    ).toEqual(['site.title'])
  })

  test('finds visible JSX and notification text but ignores comments', () => {
    const source = `
      // 这段注释不会显示
      const internalFixture = "内部固定值"
      toast.error("硬编码通知")
      export function View() { return <p>直接显示</p> }
    `
    expect(findHardcodedVisibleText(source, 'view.tsx')).toMatchObject([
      { value: '硬编码通知' },
      { value: '直接显示' },
    ])
  })

  test('supports an explicit adjacent-line allowlist marker', () => {
    const source = `
      // i18n-ignore: stable external fixture
      const fixture = "上游固定值"
    `
    expect(findHardcodedVisibleText(source, 'fixture.ts')).toEqual([])
  })

  test('finds visible English JSX text and attributes', () => {
    const source = `
      export function Brand() {
        return <div title="Product title">New API Pilot</div>
      }
    `
    expect(findHardcodedVisibleText(source, 'brand.tsx')).toMatchObject([
      { value: 'Product title' },
      { value: 'New API Pilot' },
    ])
  })

  test('finds literals nested in JSX expressions', () => {
    const source = `
      export function State({ ok }: { ok: boolean }) {
        return <p>{ok ? 'Visible English' : \`Other \${ok && 'detail'}\`}</p>
      }
    `
    expect(
      findHardcodedVisibleText(source, 'state.tsx')
        .map((violation) => violation.value)
        .sort()
    ).toEqual(['Visible English', 'Other', 'detail'].sort())
  })

  test('finds custom visible props and exempts i18next calls', () => {
    const source = `
      export function View() {
        return <Widget
          label="Visible label"
          description={ready ? 'Visible detail' : t('widget.pending')}
          message={i18n.t('widget.message')}
          value="internal-code"
        />
      }
    `
    expect(findHardcodedVisibleText(source, 'view.tsx')).toMatchObject([
      { value: 'Visible label' },
      { value: 'Visible detail' },
    ])
  })

  test('validates empty runtime-owned bootstrap metadata', () => {
    const valid = `
      <!doctype html><html><head>
        <meta name="description" content=""><title></title>
      </head><body></body></html>
    `
    expect(findBootstrapHtmlViolations(valid)).toEqual([])
    expect(
      findBootstrapHtmlViolations(`
        <!doctype html><html><head>
          <meta name="description" content="Visible detail">
          <title>Visible title</title><title></title>
        </head><body></body></html>
      `)
    ).toEqual([
      'expected exactly one <title>; found 2',
      'description meta content must be present and empty; i18next sets it at runtime',
    ])
  })

  test('collects static t and i18n.t keys and reports missing locale entries', () => {
    const scan = findTranslationKeyUsage(
      `t('customer.title'); i18n.t('account.title')`,
      'keys.ts'
    )
    expect(scan.dynamicViolations).toEqual([])
    expect(scan.staticKeys.map((usage) => usage.key)).toEqual([
      'customer.title',
      'account.title',
    ])
    expect(
      findMissingTranslationKeys(scan.staticKeys, new Set(['customer.title']))
    ).toEqual(['account.title'])
  })

  test('requires dynamic translation calls to use the controlled registry', () => {
    const invalid = findTranslationKeyUsage(
      't(`customer.status.${status}`); i18n.t(messageKey)',
      'dynamic.ts'
    )
    expect(invalid.dynamicViolations).toHaveLength(2)
    const valid = findTranslationKeyUsage(
      "t(dynamicI18nKey('account', messageKey))",
      'dynamic.ts'
    )
    expect(valid.dynamicViolations).toEqual([])
    expect(valid.dynamicRegistries.map((usage) => usage.registry)).toEqual([
      'account',
    ])
    expect(
      findUnknownDynamicRegistries(
        valid.dynamicRegistries,
        new Set(['account'])
      )
    ).toEqual([])
    expect(
      findUnknownDynamicRegistries(valid.dynamicRegistries, new Set(['site']))
    ).toEqual(['account'])
  })

  test('rejects keys outside the runtime dynamic registry', () => {
    expect(dynamicI18nKey('api', 'Request failed')).toBe('Request failed')
    expect(dynamicI18nKey('site', 'site.performance.range.24h')).toBe(
      'site.performance.range.24h'
    )
    expect(() => dynamicI18nKey('api', 'missing.dynamic.key')).toThrow(
      'Unregistered dynamic i18n key'
    )
  })

  test('registers every message code used by MessageRef translation', () => {
    for (const code of MESSAGE_CODES) {
      expect(dynamicI18nKey('api', code)).toBe(code)
    }
  })

  test('parses only static named registries', () => {
    const valid = parseDynamicI18nRegistries(`
      export const DYNAMIC_I18N_REGISTRIES = {
        account: ['account.status.active', 'Request failed'],
      } as const
    `)
    expect(valid).toEqual({
      registries: {
        account: ['account.status.active', 'Request failed'],
      },
      violations: [],
    })
    expect(
      parseDynamicI18nRegistries(`
        export const DYNAMIC_I18N_REGISTRIES = {
          account: buildKeys(),
        }
      `).violations
    ).toContain('registry "account" must be a static string array')
  })
})
