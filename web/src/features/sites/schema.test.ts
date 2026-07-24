import { describe, expect, test } from 'bun:test'

import {
  normalizeBaseUrl,
  siteDetailSearchSchema,
  siteFormSchema,
  siteSearchMiddlewares,
  sitesSearchSchema,
} from './schema'

describe('site schemas', () => {
  test('normalizes a safe HTTP base URL', () => {
    expect(normalizeBaseUrl(' HTTPS://EXAMPLE.COM/api/// ')).toBe(
      'https://example.com/api'
    )
    expect(
      siteFormSchema.parse({
        baseUrl: 'https://example.com/',
        name: ' 华东站点 ',
        remark: '',
      })
    ).toEqual({ baseUrl: 'https://example.com', name: '华东站点', remark: '' })
  })

  test('rejects credentials and URL metadata', () => {
    expect(() => normalizeBaseUrl('https://root:secret@example.com')).toThrow()
    expect(() => normalizeBaseUrl('https://example.com?token=secret')).toThrow()
    expect(() => normalizeBaseUrl('file:///tmp/site')).toThrow()
  })

  test('restores a single URL filter as an array', () => {
    expect(
      sitesSearchSchema.parse({ management: 'active', online: ['offline'] })
    ).toMatchObject({ management: ['active'], online: ['offline'] })
  })

  test('strips empty site filters from the browser URL search state', () => {
    const search = sitesSearchSchema.parse({})
    const normalized = siteSearchMiddlewares[0]({
      next: (nextSearch) => nextSearch,
      search,
    })

    expect(normalized as Record<string, unknown>).toEqual({})
  })

  test('keeps active site filters in the browser URL search state', () => {
    const search = sitesSearchSchema.parse({ management: 'active' })
    const normalized = siteSearchMiddlewares[0]({
      next: (nextSearch) => nextSearch,
      search,
    })

    expect(normalized as Record<string, unknown>).toEqual({
      management: ['active'],
    })
  })

  test('normalizes numeric run deep links to bigint-safe strings', () => {
    expect(siteDetailSearchSchema.parse({ runId: 10 }).runId).toBe('10')
  })
})
