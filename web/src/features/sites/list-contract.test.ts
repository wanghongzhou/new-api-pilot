import { describe, expect, test } from 'bun:test'

import { siteListParams } from './list-contract'
import type { SiteSearch } from './types'

const defaultSearch: SiteSearch = {
  auth: [],
  filter: '',
  health: [],
  management: [],
  online: [],
  order: 'desc',
  page: 1,
  pageSize: 20,
  sort: 'priority',
  statistics: [],
  view: 'card',
}

describe('site list contract', () => {
  test('omits every request parameter for the default unfiltered list', () => {
    expect(siteListParams(defaultSearch)).toEqual({
      auth_status: undefined,
      health_status: undefined,
      keyword: undefined,
      management_status: undefined,
      online_status: undefined,
      p: undefined,
      page_size: undefined,
      sort_by: undefined,
      sort_order: undefined,
      statistics_status: undefined,
    })
  })

  test('keeps only filters and list options that differ from defaults', () => {
    expect(
      siteListParams({
        ...defaultSearch,
        filter: 'east',
        online: ['offline'],
        order: 'asc',
        page: 2,
        pageSize: 50,
        sort: 'name',
      })
    ).toEqual({
      auth_status: undefined,
      health_status: undefined,
      keyword: 'east',
      management_status: undefined,
      online_status: ['offline'],
      p: 2,
      page_size: 50,
      sort_by: 'name',
      sort_order: 'asc',
      statistics_status: undefined,
    })
  })
})
