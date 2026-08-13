import type {
  DataStatus,
  DecimalString,
  IdString,
  MetricString,
  Timestamp,
} from '@/lib/api-types'

export type PerformanceMetricSource = 'official_average' | 'counter_ready'
export type PerformanceAggregationStatus = 'complete' | 'unavailable'
export interface PerformanceCounters {
  request_count: MetricString | null
  success_count: MetricString | null
  total_latency_ms: MetricString | null
  ttft_sum_ms: MetricString | null
  ttft_count: MetricString | null
  output_tokens: MetricString | null
  generation_ms: MetricString | null
}
export interface PerformanceHistoryItem extends PerformanceCounters {
  id: IdString
  site_id: IdString
  site_name: string
  model_name: string
  group: string
  bucket_start: Timestamp
  series_schema: string
  metric_source: PerformanceMetricSource
  avg_ttft_ms: DecimalString | null
  avg_latency_ms: DecimalString | null
  success_rate: DecimalString | null
  avg_tps: DecimalString | null
  collected_at: Timestamp
}
export interface PerformanceHistoryPage {
  items: PerformanceHistoryItem[]
  total: MetricString
  page: number
  page_size: number
  data_status: DataStatus
  as_of: Timestamp | null
  completeness: PerformanceCompleteness
}
export interface PerformanceCompleteness {
  data_status: DataStatus
  successful_site_count: number
  unavailable_site_count: number
  expected_site_count: number
}
export interface PerformanceWeightedMetric {
  success_rate: DecimalString | null
  avg_latency_ms: DecimalString | null
  avg_ttft_ms: DecimalString | null
  avg_tps: DecimalString | null
  request_count: MetricString | null
}
export interface PerformanceDimensionBreakdown extends PerformanceWeightedMetric {
  dimension: string
}
export interface PerformanceHistoryStatisticsResponse {
  summary: PerformanceWeightedMetric
  trend: PerformanceHistoryItem[]
  model_breakdown: PerformanceDimensionBreakdown[]
  group_breakdown: PerformanceDimensionBreakdown[]
  site_breakdown: PerformanceHistoryItem[]
  aggregation_status: PerformanceAggregationStatus
  data_status: DataStatus
  as_of: Timestamp | null
  completeness: PerformanceCompleteness
  unavailable_reason?: string
}
export interface PerformanceHistoryQueryParams {
  p: number
  page_size: number
  start_timestamp: Timestamp
  end_timestamp: Timestamp
  site_ids?: IdString[]
  model_names?: string[]
  groups?: string[]
}
