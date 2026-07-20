import type {
  BackfillSummary,
  Completeness,
  UsageSummary,
} from '@/features/sites/types'
import type {
  IdString,
  ListQuery,
  MetricString,
  PageData,
  RateInfo,
  Timestamp,
} from '@/lib/api-types'

export type AccountRemoteState = 'normal' | 'missing' | 'identity_mismatch'
export type AccountManagedStatus = 'active' | 'archived'
export type RemoteUserStatusFilter = '1' | '2'

export interface AccountListItem {
  id: IdString
  site_id: IdString
  site_name: string
  customer_id: IdString
  customer_name: string
  remote_user_id: IdString
  remote_created_at: Timestamp
  username: string
  display_name: string
  remote_group: string
  remote_status: number
  remote_state: AccountRemoteState
  managed_status: AccountManagedStatus
  quota: MetricString
  used_quota: MetricString
  request_count: MetricString
  rate: RateInfo
  last_synced_at: Timestamp | null
  today: UsageSummary
  backfill: BackfillSummary
  updated_at: Timestamp
}

export interface AccountDetail extends AccountListItem {
  remark: string
  remote_missing_count: number
  last_remote_seen_at: Timestamp | null
  statistics_paused_at: Timestamp | null
  completeness: Completeness
  created_at: Timestamp
}

export interface RemoteUserItem {
  id: IdString
  username: string
  display_name: string
  role: number
  status: number
  group: string
  quota: MetricString
  used_quota: MetricString
  request_count: MetricString
  created_at: Timestamp
  last_login_at: Timestamp | null
  already_managed: boolean
  managed_account_id: IdString | null
  managed_customer_name: string
}

export interface AccountCreateRequest {
  site_id: IdString
  customer_id: IdString
  remote_user_id: IdString
  remark?: string
}

export interface AccountUpdateRequest {
  remark: string
}

export interface AccountSearch {
  page: number
  pageSize: number
  filter: string
  siteId?: IdString
  customerId?: IdString
  remoteStatus: RemoteUserStatusFilter[]
  remoteState: AccountRemoteState[]
  managedStatus: AccountManagedStatus[]
  sort: 'updated_at' | 'username' | 'today_quota' | 'quota'
  order: 'asc' | 'desc'
}

export interface AccountListParams extends ListQuery {
  site_id?: IdString
  customer_id?: IdString
  remote_status?: RemoteUserStatusFilter[]
  remote_state?: AccountRemoteState[]
  managed_status?: AccountManagedStatus[]
}

export interface RemoteUserListParams extends ListQuery {
  keyword: string
}

export type AccountPage = PageData<AccountListItem>
export type RemoteUserPage = PageData<RemoteUserItem>
