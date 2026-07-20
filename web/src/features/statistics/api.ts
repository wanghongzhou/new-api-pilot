import { api, requestApiData } from '@/lib/api'
import type { IdString, PageData } from '@/lib/api-types'

import { parseExportDownloadResponse } from './export-download'
import type {
  ChannelOptionPage,
  GroupOptionPage,
  ModelOptionPage,
  NodeOptionPage,
  StatisticsBreakdownByScope,
  StatisticsExportCreateRequest,
  StatisticsExportDownload,
  StatisticsExportJobItem,
  StatisticsExportListParams,
  StatisticsOptionParams,
  StatisticsQueryParams,
  StatisticsResponse,
  StatisticsScope,
  TokenOptionPage,
} from './types'

type SearchValue = number | string | readonly string[] | null | undefined

function statisticsSearchParams(values: Record<string, SearchValue>) {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(values)) {
    if (value == null || value === '') continue
    if (Array.isArray(value)) {
      if (value.length === 0) continue
      if (
        key === 'model_names' ||
        key === 'channel_keys' ||
        key === 'use_groups' ||
        key === 'token_keys' ||
        key === 'node_names' ||
        key === 'status'
      ) {
        for (const item of value) params.append(key, item)
      } else {
        params.set(key, value.join(','))
      }
      continue
    }
    params.set(key, String(value))
  }
  return params
}

const statisticsPath: Record<StatisticsScope, string> = {
  account: 'accounts',
  channel: 'channels',
  customer: 'customers',
  global: 'global',
  group: 'groups',
  model: 'models',
  node: 'nodes',
  site: 'sites',
  token: 'tokens',
}

export function getStatistics<TScope extends StatisticsScope>(
  scope: TScope,
  params: StatisticsQueryParams
): Promise<StatisticsResponse<StatisticsBreakdownByScope[TScope]>> {
  return requestApiData<StatisticsResponse<StatisticsBreakdownByScope[TScope]>>(
    {
      method: 'get',
      params: statisticsSearchParams({
        account_ids: params.account_ids,
        channel_keys: params.channel_keys,
        customer_ids: params.customer_ids,
        end_timestamp: params.end_timestamp,
        granularity: params.granularity,
        model_names: params.model_names,
        node_names: params.node_names,
        p: params.p,
        page_size: params.page_size,
        site_ids: params.site_ids,
        sort_by: params.sort_by,
        sort_order: params.sort_order,
        start_timestamp: params.start_timestamp,
        token_keys: params.token_keys,
        use_groups: params.use_groups,
      }),
      url: `/api/statistics/${statisticsPath[scope]}`,
    }
  )
}

export function listModelOptions(
  values: StatisticsOptionParams
): Promise<ModelOptionPage> {
  return requestApiData<ModelOptionPage>({
    method: 'get',
    params: statisticsSearchParams({
      keyword: values.keyword,
      p: values.p ?? 1,
      page_size: values.page_size ?? 20,
      site_ids: values.site_ids,
    }),
    url: '/api/statistics/options/models',
  })
}

export function listChannelOptions(
  values: StatisticsOptionParams
): Promise<ChannelOptionPage> {
  return requestApiData<ChannelOptionPage>({
    method: 'get',
    params: statisticsSearchParams({
      keyword: values.keyword,
      p: values.p ?? 1,
      page_size: values.page_size ?? 20,
      site_ids: values.site_ids,
    }),
    url: '/api/statistics/options/channels',
  })
}

export function listGroupOptions(
  values: StatisticsOptionParams
): Promise<GroupOptionPage> {
  return requestApiData<GroupOptionPage>({
    method: 'get',
    params: statisticsSearchParams({
      keyword: values.keyword,
      p: values.p ?? 1,
      page_size: values.page_size ?? 20,
      site_ids: values.site_ids,
    }),
    url: '/api/statistics/options/groups',
  })
}

export function listTokenOptions(
  values: StatisticsOptionParams
): Promise<TokenOptionPage> {
  return requestApiData<TokenOptionPage>({
    method: 'get',
    params: statisticsSearchParams({
      keyword: values.keyword,
      p: values.p ?? 1,
      page_size: values.page_size ?? 20,
      site_ids: values.site_ids,
    }),
    url: '/api/statistics/options/tokens',
  })
}

export function listNodeOptions(
  values: StatisticsOptionParams
): Promise<NodeOptionPage> {
  return requestApiData<NodeOptionPage>({
    method: 'get',
    params: statisticsSearchParams({
      keyword: values.keyword,
      p: values.p ?? 1,
      page_size: values.page_size ?? 20,
      site_ids: values.site_ids,
    }),
    url: '/api/statistics/options/nodes',
  })
}

export function createStatisticsExport(
  request: StatisticsExportCreateRequest
): Promise<StatisticsExportJobItem> {
  return requestApiData<StatisticsExportJobItem>({
    data: request,
    method: 'post',
    url: '/api/statistics/export',
  })
}

export function getStatisticsExport(
  id: IdString
): Promise<StatisticsExportJobItem> {
  return requestApiData<StatisticsExportJobItem>({
    method: 'get',
    url: `/api/statistics/exports/${id}`,
  })
}

export function listStatisticsExports(
  params: StatisticsExportListParams
): Promise<PageData<StatisticsExportJobItem>> {
  return requestApiData({
    method: 'get',
    params: statisticsSearchParams({
      format: params.format,
      p: params.p,
      page_size: params.page_size,
      sort_by: params.sort_by,
      sort_order: params.sort_order,
      statistics_type: params.statistics_type,
      status: params.status,
    }),
    url: '/api/statistics/exports',
  })
}

export async function downloadStatisticsExport(
  job: Pick<StatisticsExportJobItem, 'file_name' | 'format' | 'id'>
): Promise<StatisticsExportDownload> {
  const response = await api.get<Blob>(
    `/api/statistics/exports/${job.id}/download`,
    {
      disableDedupe: true,
      responseType: 'blob',
      validateStatus: (status) =>
        (status >= 200 && status < 300) || status === 410,
    }
  )
  return parseExportDownloadResponse(response, {
    exportId: job.id,
    fileName: job.file_name,
    format: job.format,
  })
}
