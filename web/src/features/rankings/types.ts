import type {
  DataStatus,
  DecimalString,
  IdString,
  MetricString,
  NonNegativeIdString,
  Timestamp,
} from '@/lib/api-types'

export type RankingPeriod = 'today' | 'week' | 'month' | 'year'
export type RankingTab = 'models' | 'vendors'

export interface RankingItem {
  dimension_id: string
  dimension_name: string
  token_used: MetricString
  request_count: MetricString
  quota: MetricString
  share: DecimalString
  growth: DecimalString | null
  rank: number
}

export interface RankingHistoryPoint {
  dimension_id: string
  bucket_start: Timestamp
  token_used: MetricString
}

export interface RankingSiteBreakdown {
  dimension_id: string
  site_id: IdString
  site_name: string
  token_used: MetricString
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface LocalRankingResponse {
  period: RankingPeriod
  start_timestamp: Timestamp
  end_timestamp: Timestamp
  items: RankingItem[]
  movers: RankingItem[]
  droppers: RankingItem[]
  history: RankingHistoryPoint[]
  site_breakdown: RankingSiteBreakdown[]
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface RankingQueryParams {
  period: RankingPeriod
  site_ids?: IdString[]
}

export type VendorDimensionId = NonNegativeIdString
