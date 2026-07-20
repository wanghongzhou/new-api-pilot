import { requestApiData } from '@/lib/api'

import type { ChangePasswordRequest, LoginRequest, LoginUser } from './types'

export function login(request: LoginRequest): Promise<LoginUser> {
  return requestApiData<LoginUser>({
    data: request,
    method: 'post',
    skipAuthHandling: true,
    url: '/api/user/login',
  })
}

export function logout(): Promise<null> {
  return requestApiData<null>({ method: 'post', url: '/api/user/logout' })
}

export function getSelf(): Promise<LoginUser> {
  return requestApiData<LoginUser>({
    method: 'get',
    skipAuthHandling: true,
    url: '/api/user/self',
  })
}

export function changePassword(request: ChangePasswordRequest): Promise<null> {
  return requestApiData<null>({
    data: request,
    method: 'put',
    url: '/api/user/password',
  })
}
