import type {
  DataStatus,
  DecimalString,
  IdString,
  MetricString,
  Timestamp,
} from '@/lib/api-types'

export type PricingCatalogTab = 'groups' | 'pricing'
export type PricingCatalogState = 'normal' | 'missing'
export type PricingBillingMode = 'token' | 'fixed' | 'tiered_expr'

export interface PricingCatalogItem {
  id: IdString
  site_id: IdString
  vendor_id: IdString
  quota_type: MetricString
  site_name: string
  model_name: string
  vendor_name: string
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
  billing_mode: string
  billing_expr: string
  pricing_source: 'token_default' | 'token_explicit' | 'fixed' | 'tiered_expr'
  ability_available: boolean
  enable_groups: string[]
  supported_endpoint_types: string[]
  pricing_version: string
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
  topup_ratio: DecimalString | null
  description: string
  user_selectable: boolean
  default_use_auto_group: boolean
  auto_priority: number | null
  outgoing_overrides: Record<string, DecimalString>
  incoming_overrides: Record<string, DecimalString>
  visible_to_groups: Record<string, string>
  hidden_from_groups: string[]
  remote_state: PricingCatalogState
  missing_count: number
  active_pricing_count: MetricString
  missing_pricing_count: MetricString
  model_names: string[]
  missing_model_names: string[]
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
  site_count: MetricString
  pricing_active: MetricString
  pricing_missing: MetricString
  group_active: MetricString
  group_missing: MetricString
  data_status: DataStatus
  sites: PricingCatalogSiteOverview[]
}

export interface PricingCatalogSiteOverview {
  site_id: IdString
  site_name: string
  pricing_active: MetricString
  pricing_missing: MetricString
  group_active: MetricString
  group_missing: MetricString
  pricing_data_status: DataStatus
  group_data_status: DataStatus
  pricing_as_of: Timestamp | null
  group_as_of: Timestamp | null
}

export interface PricingCatalogQueryParams {
  p: number
  page_size: number
  site_ids?: IdString[]
  states?: PricingCatalogState[]
  keyword?: string
  group?: string
  billing_mode?: PricingBillingMode
}
