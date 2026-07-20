import type { IdString } from '@/lib/api-types'

export type PlatformRole = 'admin' | 'viewer'
export type PlatformUserStatus = 1 | 2

export interface LoginUser {
  id: IdString
  username: string
  display_name: string
  role: PlatformRole
  status: PlatformUserStatus
  must_change_password: boolean
}

export interface LoginRequest {
  username: string
  password: string
}

export interface ChangePasswordRequest {
  original_password: string
  new_password: string
}
