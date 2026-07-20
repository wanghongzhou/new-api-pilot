import type {
  DataStatus,
  DecimalString,
  IdString,
  MetricString,
  NonNegativeIdString,
  Timestamp,
} from '@/lib/api-types'

export type FinancialOperationTab = 'topups' | 'redemptions'
export type FinanceRemoteState = 'normal' | 'missing'

export interface FinanceInventoryQueryParams {
  p: number
  page_size: number
  site_ids?: IdString[]
  remote_id?: IdString
  remote_user_id?: NonNegativeIdString
  statuses?: string[]
  providers?: string[]
  methods?: string[]
  states?: FinanceRemoteState[]
  start_timestamp?: Timestamp
  end_timestamp?: Timestamp
  keyword?: string
}

interface FinanceInventoryBase {
  id: IdString
  site_id: IdString
  remote_id: IdString
  remote_user_id: NonNegativeIdString
  site_name: string
  remote_state: FinanceRemoteState
  missing_count: number
  first_seen_at: Timestamp
  last_seen_at: Timestamp | null
}

export interface TopupInventoryItem extends FinanceInventoryBase {
  amount: MetricString
  money: DecimalString
  payment_method: string
  payment_provider: string
  create_time: Timestamp
  complete_time: Timestamp
  status: string
}

export interface RedemptionInventoryItem extends FinanceInventoryBase {
  name: string
  status: number
  derived_status: string
  quota: MetricString
  created_time: Timestamp
  redeemed_time: Timestamp
  used_user_id: NonNegativeIdString
  expired_time: Timestamp
}

export interface FinanceInventoryPage<TItem> {
  items: TItem[]
  total: number
  page: number
  page_size: number
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface FinanceMetric {
  count: MetricString
  missing_count: MetricString
  amount?: MetricString
  money?: DecimalString
  quota?: MetricString
}

export interface FinanceBreakdown extends FinanceMetric {
  dimension_id: string
  dimension_name: string
  site_id: IdString | ''
  site_name: string
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface FinanceStatisticsResponse {
  summary: FinanceMetric
  status_breakdown: FinanceBreakdown[]
  provider_breakdown?: FinanceBreakdown[]
  site_breakdown: FinanceBreakdown[]
  data_status: DataStatus
}
