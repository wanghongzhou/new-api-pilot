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
  avg_ttft_ms: DecimalString
  avg_latency_ms: DecimalString
  success_rate: DecimalString
  avg_tps: DecimalString
  collected_at: Timestamp
}
export interface PerformanceHistoryPage {
  items: PerformanceHistoryItem[]
  total: number
  page: number
  page_size: number
  data_status: DataStatus
  as_of: Timestamp | null
}
export interface PerformanceWeightedMetric {
  success_rate: DecimalString | null
  avg_latency_ms: DecimalString | null
  avg_ttft_ms: DecimalString | null
  avg_tps: DecimalString | null
  request_count: MetricString | null
}
export interface PerformanceHistoryStatisticsResponse {
  summary: PerformanceWeightedMetric
  trend: PerformanceHistoryItem[]
  site_breakdown: PerformanceHistoryItem[]
  aggregation_status: PerformanceAggregationStatus
  data_status: DataStatus
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
