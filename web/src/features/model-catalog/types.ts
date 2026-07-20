import type {
  DataStatus,
  IdString,
  MetricString,
  NonNegativeIdString,
  Timestamp,
} from '@/lib/api-types'

export type ModelCatalogTab = 'catalog' | 'coverage' | 'missing'
export type ModelBinaryState = 0 | 1
export type ModelNameRule = 0 | 1 | 2 | 3

export interface ModelCatalogItem {
  id: IdString
  site_id: IdString
  remote_id: IdString
  site_name: string
  model_name: string
  description: string
  icon: string
  tags: string
  vendor_id: NonNegativeIdString
  status: ModelBinaryState
  sync_official: ModelBinaryState
  name_rule: ModelNameRule
  created_time: Timestamp
  updated_time: Timestamp
  covered_channels: MetricString
  covered_groups: MetricString
  data_status: DataStatus
}

export interface ModelCatalogPage {
  items: ModelCatalogItem[]
  total: number
  page: number
  page_size: number
  data_status: DataStatus
}

export interface ModelCoverageMetric {
  catalog_models: MetricString
  exact_covered_models: MetricString
  exact_missing_models: MetricString
  channel_mappings: MetricString
}

export interface ModelCoverageBreakdown extends ModelCoverageMetric {
  dimension_id: string
  dimension_name: string
  site_id: NonNegativeIdString
  site_name: string
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface ModelCoverageResponse extends ModelCoverageMetric {
  data_status: DataStatus
  site_breakdown: ModelCoverageBreakdown[]
  vendor_breakdown: ModelCoverageBreakdown[]
  status_breakdown: ModelCoverageBreakdown[]
}

export interface MissingModelItem {
  site_id: IdString
  site_name: string
  remote_channel_id: NonNegativeIdString
  channel_name: string
  model_name: string
  group: string
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface MissingModelPage {
  items: MissingModelItem[]
  total: number
  page: number
  page_size: number
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface ModelCatalogQueryParams {
  p: number
  page_size: number
  site_ids?: IdString[]
  vendor_id?: NonNegativeIdString
  statuses?: ModelBinaryState[]
  sync_official?: ModelBinaryState[]
  keyword?: string
}
