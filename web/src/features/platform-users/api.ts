import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  CreatePlatformUserRequest,
  PlatformUserItem,
  PlatformUserListParams,
  PlatformUserPage,
  ResetPasswordRequest,
  UpdatePlatformUserRequest,
} from './types'

export function listPlatformUsers(
  params: PlatformUserListParams
): Promise<PlatformUserPage> {
  return requestApiData<PlatformUserPage>({
    method: 'get',
    params,
    url: '/api/user/',
  })
}

export function createPlatformUser(
  request: CreatePlatformUserRequest
): Promise<PlatformUserItem> {
  return requestApiData<PlatformUserItem>({
    data: request,
    method: 'post',
    url: '/api/user/',
  })
}

export function updatePlatformUser(
  id: IdString,
  request: UpdatePlatformUserRequest
): Promise<PlatformUserItem> {
  return requestApiData<PlatformUserItem>({
    data: request,
    method: 'put',
    url: `/api/user/${id}`,
  })
}

export function enablePlatformUser(id: IdString): Promise<null> {
  return requestApiData<null>({
    method: 'post',
    url: `/api/user/${id}/enable`,
  })
}

export function disablePlatformUser(id: IdString): Promise<null> {
  return requestApiData<null>({
    method: 'post',
    url: `/api/user/${id}/disable`,
  })
}

export function resetPlatformUserPassword(
  id: IdString,
  request: ResetPasswordRequest
): Promise<null> {
  return requestApiData<null>({
    data: request,
    method: 'post',
    url: `/api/user/${id}/reset-password`,
  })
}
