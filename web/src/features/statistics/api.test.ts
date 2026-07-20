import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import { api } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'

import {
  createStatisticsExport,
  downloadStatisticsExport,
  getStatisticsExport,
  getStatistics,
  listChannelOptions,
  listGroupOptions,
  listModelOptions,
  listNodeOptions,
  listStatisticsExports,
  listTokenOptions,
} from './api'
import { buildEntityExportRequest } from './export-request'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

describe('statistics export API', () => {
  test('posts a frozen customer filter set using the 05C export contract', async () => {
    api.defaults.adapter = (async (config) => {
      expect(config.url).toBe('/api/statistics/export')
      expect(config.method).toBe('post')
      expect(JSON.parse(String(config.data))).toEqual({
        filters: {
          account_ids: [],
          channel_keys: [],
          customer_ids: ['9007199254740993'],
          end_timestamp: 1_783_875_600,
          granularity: 'hour',
          model_names: [],
          node_names: [],
          site_ids: [],
          sort_by: 'name',
          sort_order: 'asc',
          start_timestamp: 1_783_789_200,
          token_keys: [],
          use_groups: [],
        },
        format: 'xlsx',
        statistics_type: 'customer',
      })
      return {
        config,
        data: {
          code: '',
          data: {},
          message: '',
          request_id: 'req_export',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter

    await createStatisticsExport(
      buildEntityExportRequest(
        'customer',
        parseIdString('9007199254740993'),
        'xlsx',
        {
          accountIds: [],
          channelKeys: [],
          customerIds: [],
          display: 'usd',
          end: 1_783_875_600,
          granularity: 'hour',
          metric: 'quota',
          models: [],
          nodeNames: [],
          order: 'asc',
          page: 4,
          pageSize: 50,
          sort: 'bucket_start',
          siteIds: [],
          start: 1_783_789_200,
          tokenKeys: [],
          useGroups: [],
          view: 'table',
        }
      )
    )
  })

  test('serializes ID lists once and model/channel keys as repeated values', async () => {
    const urls: string[] = []
    api.defaults.adapter = (async (config) => {
      const params = config.params as URLSearchParams
      urls.push(`${config.url}?${params.toString()}`)
      return {
        config,
        data: {
          code: '',
          data: {},
          message: '',
          request_id: 'req_statistics',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter

    await getStatistics('model', {
      end_timestamp: 1_783_875_600,
      granularity: 'hour',
      model_names: ['模型-A', 'Model-A'],
      p: 2,
      page_size: 20,
      site_ids: [
        parseIdString('9007199254740993'),
        parseIdString('9007199254740995'),
      ],
      sort_by: 'quota',
      sort_order: 'desc',
      start_timestamp: 1_783_789_200,
    })
    await listModelOptions({
      keyword: '长模型名称',
      site_ids: [parseIdString('9007199254740993')],
    })
    await listChannelOptions({
      keyword: '通道',
      site_ids: [parseIdString('9007199254740993')],
    })

    expect(urls[0]).toContain('/api/statistics/models?')
    expect(urls[0]).toContain('site_ids=9007199254740993%2C9007199254740995')
    expect(urls[0]).toContain('model_names=%E6%A8%A1%E5%9E%8B-A')
    expect(urls[0]).toContain('model_names=Model-A')
    expect(urls[1]).toContain('/api/statistics/options/models?')
    expect(urls[2]).toContain('/api/statistics/options/channels?')
  })

  test('uses flow parity endpoints and preserves empty and bigint-safe identities', async () => {
    const urls: string[] = []
    api.defaults.adapter = (async (config) => {
      urls.push(`${config.url}?${String(config.params)}`)
      return {
        config,
        data: {
          code: '',
          data: {},
          message: '',
          request_id: 'req_flow_statistics',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter

    const common = {
      end_timestamp: 1_783_875_600,
      granularity: 'hour' as const,
      p: 1,
      page_size: 20,
      site_ids: [parseIdString('9007199254740993')],
      sort_by: 'name' as const,
      sort_order: 'asc' as const,
      start_timestamp: 1_783_789_200,
    }
    await getStatistics('group', { ...common, use_groups: ['', 'vip'] })
    await getStatistics('token', {
      ...common,
      token_keys: ['9007199254740993:0'],
    })
    await getStatistics('node', { ...common, node_names: ['', 'Node-A'] })
    await listGroupOptions({ site_ids: common.site_ids })
    await listTokenOptions({ site_ids: common.site_ids })
    await listNodeOptions({ site_ids: common.site_ids })

    expect(urls[0]).toContain('/api/statistics/groups?')
    expect(urls[0]).toContain('use_groups=&use_groups=vip')
    expect(urls[1]).toContain('/api/statistics/tokens?')
    expect(urls[1]).toContain('token_keys=9007199254740993%3A0')
    expect(urls[2]).toContain('/api/statistics/nodes?')
    expect(urls[2]).toContain('node_names=&node_names=Node-A')
    expect(urls.slice(3).map((url) => url.split('?')[0])).toEqual([
      '/api/statistics/options/groups',
      '/api/statistics/options/tokens',
      '/api/statistics/options/nodes',
    ])
  })

  test('uses the current-user export list, detail, and download endpoints', async () => {
    const requests: Parameters<AxiosAdapter>[0][] = []
    api.defaults.adapter = (async (config) => {
      requests.push(config)
      const download = config.url?.endsWith('/download')
      return {
        config,
        data: download
          ? new Blob(['csv'])
          : {
              code: '',
              data: {},
              message: '',
              request_id: 'req_exports',
              success: true,
            },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }) as AxiosAdapter

    const id = parseIdString('9007199254740993')
    await listStatisticsExports({
      format: 'csv',
      p: 2,
      page_size: 50,
      sort_by: 'file_size',
      sort_order: 'asc',
      statistics_type: 'model',
      status: ['success', 'failed'],
    })
    await getStatisticsExport(id)
    const result = await downloadStatisticsExport({
      file_name: 'model-statistics.csv',
      format: 'csv',
      id,
    })

    expect(requests.map(({ method, url }) => [method, url])).toEqual([
      ['get', '/api/statistics/exports'],
      ['get', '/api/statistics/exports/9007199254740993'],
      ['get', '/api/statistics/exports/9007199254740993/download'],
    ])
    const exportParams = requests[0]?.params as URLSearchParams
    expect(Object.fromEntries(exportParams)).toEqual({
      format: 'csv',
      p: '2',
      page_size: '50',
      sort_by: 'file_size',
      sort_order: 'asc',
      statistics_type: 'model',
      status: 'failed',
    })
    expect(exportParams.getAll('status')).toEqual(['success', 'failed'])
    expect(requests[2]?.responseType).toBe('blob')
    expect(requests[2]?.disableDedupe).toBeTrue()
    expect(result.fileName).toBe('model-statistics.csv')
  })
})
