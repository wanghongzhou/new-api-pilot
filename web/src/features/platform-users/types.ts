import type { IdString, PageData, Timestamp } from '@/lib/api-types'

import type { PlatformRole, PlatformUserStatus } from '../auth/types'

export interface PlatformUserItem {
  id: IdString
  username: string
  display_name: string
  role: PlatformRole
  status: PlatformUserStatus
  must_change_password: boolean
  last_login_at: Timestamp | null
  created_at: Timestamp
  updated_at: Timestamp
}

export interface PlatformUserListParams {
  p: number
  page_size: number
  keyword?: string
  role?: PlatformRole
  status?: PlatformUserStatus
  sort_by?: 'created_at' | 'username' | 'last_login_at'
  sort_order?: 'asc' | 'desc'
}

export type PlatformUserPage = PageData<PlatformUserItem>

export interface CreatePlatformUserRequest {
  username: string
  display_name: string
  role: PlatformRole
  password: string
}

export interface UpdatePlatformUserRequest {
  username: string
  display_name: string
  role: PlatformRole
}

export interface ResetPasswordRequest {
  new_password: string
}

export interface PlatformUserSearch {
  filter: string
  page: number
  pageSize: number
  role?: PlatformRole
  status?: PlatformUserStatus
}
