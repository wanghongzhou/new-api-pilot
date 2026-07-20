import type {
  BackfillSummary,
  Completeness,
  UsageSummary,
} from '@/features/sites/types'
import type { SiteQuotaBreakdown } from '@/features/statistics/types'
import type { IdString, ListQuery, PageData, Timestamp } from '@/lib/api-types'

export type CustomerStatus = 'communicating' | 'signing' | 'using' | 'disabled'

export interface CustomerListItem {
  id: IdString
  name: string
  contact: string
  remark: string
  status: CustomerStatus
  account_count: number
  active_account_count: number
  archived_account_count: number
  site_count: number
  today: UsageSummary & { site_breakdown: SiteQuotaBreakdown[] }
  backfill: BackfillSummary
  updated_at: Timestamp
}

export interface CustomerDetail extends CustomerListItem {
  statistics_paused_at: Timestamp | null
  completeness: Completeness
  created_at: Timestamp
}

export interface CustomerCreateRequest {
  name: string
  contact?: string
  remark?: string
  status: Exclude<CustomerStatus, 'disabled'>
}

export type CustomerUpdateRequest = CustomerCreateRequest

export interface CustomerSearch {
  page: number
  pageSize: number
  filter: string
  status: CustomerStatus[]
  view: 'card' | 'table'
  sort: 'updated_at' | 'name' | 'today_quota' | 'account_count'
  order: 'asc' | 'desc'
}

export interface CustomerListParams extends ListQuery {
  status?: CustomerStatus[]
}

export type CustomerPage = PageData<CustomerListItem>
