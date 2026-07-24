import type {
  DataStatus,
  DecimalString,
  IdString,
  MetricString,
  Timestamp,
} from '@/lib/api-types'

export type PricingCatalogTab =
  | 'pricing'
  | 'groups'
  | 'site-analysis'
  | 'vendor-analysis'
  | 'group-model-analysis'
  | 'group-availability-analysis'
export type PricingCatalogState = 'normal' | 'missing'

export interface PricingCatalogItem {
  id: IdString
  site_id: IdString
  vendor_id: IdString
  quota_type: MetricString
  site_name: string
  model_name: string
  vendor_key: string
  description: string
  icon: string
  tags: string
  owner_by: string
  model_ratio: DecimalString
  model_price: DecimalString
  completion_ratio: DecimalString
  cache_ratio: DecimalString | null
  create_cache_ratio: DecimalString | null
  image_ratio: DecimalString | null
  audio_ratio: DecimalString | null
  audio_completion_ratio: DecimalString | null
  enable_groups: string[]
  supported_endpoint_types: string[]
  pricing_version: string
  root_visible: boolean
  remote_state: PricingCatalogState
  missing_count: number
  collected_at: Timestamp
  data_status: DataStatus
}

export interface PricingGroupItem {
  id: IdString
  site_id: IdString
  site_name: string
  name: string
  ratio: DecimalString | null
  description: string
  root_visible: boolean
  remote_state: PricingCatalogState
  missing_count: number
  collected_at: Timestamp
  data_status: DataStatus
}

export interface CatalogPage<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  data_status: DataStatus
}

export interface PricingGroupPage extends CatalogPage<PricingGroupItem> {
  as_of: Timestamp | null
  site_breakdown: PricingCatalogSiteBreakdown[]
}

export interface PricingCatalogSiteBreakdown {
  site_id: IdString
  site_name: string
  total: MetricString
  missing: MetricString
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface PricingCatalogStatistics {
  total: MetricString
  missing: MetricString
  group_total: MetricString
  data_status: DataStatus
  site_breakdown: PricingCatalogSiteBreakdown[]
  vendor_breakdown: PricingVendorBreakdown[]
  group_breakdown: PricingModelGroupBreakdown[]
  group_catalog_breakdown: GroupCatalogAvailabilityBreakdown[]
}

export interface PricingVendorBreakdown {
  vendor_key: string
  vendor_id: IdString
  total: MetricString
  missing: MetricString
}

export interface PricingModelGroupBreakdown {
  group_name: string
  model_count: MetricString
}

export interface GroupCatalogAvailabilityBreakdown {
  root_visible: boolean
  ratio_available: boolean
  count: MetricString
}

export interface PricingCatalogQueryParams {
  p: number
  page_size: number
  site_ids?: IdString[]
  states?: PricingCatalogState[]
  keyword?: string
  group?: string
}
