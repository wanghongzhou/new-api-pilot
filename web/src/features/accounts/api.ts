import type { CollectionRunItem } from '@/features/sites/types'
import type {
  AccountStatisticsBreakdown,
  EntityStatisticsParams,
  StatisticsResponse,
} from '@/features/statistics/types'
import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  AccountCreateRequest,
  AccountDetail,
  AccountListParams,
  AccountPage,
  AccountUpdateRequest,
  RemoteUserListParams,
  RemoteUserPage,
} from './types'

type SearchValue = string | number | readonly string[] | undefined

function toSearchParams(values: object): URLSearchParams {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(values) as [
    string,
    SearchValue,
  ][]) {
    if (value === undefined || value === '') continue
    if (Array.isArray(value)) {
      for (const item of value) params.append(key, item)
    } else {
      params.set(key, String(value))
    }
  }
  return params
}

export function listAccounts(params: AccountListParams): Promise<AccountPage> {
  return requestApiData<AccountPage>({
    method: 'get',
    params: toSearchParams(params),
    url: '/api/accounts',
  })
}

export function listRemoteUsers(
  siteId: IdString,
  params: RemoteUserListParams
): Promise<RemoteUserPage> {
  return requestApiData<RemoteUserPage>({
    method: 'get',
    params: toSearchParams(params),
    url: `/api/accounts/site/${siteId}/remote-users`,
  })
}

export function createAccount(
  request: AccountCreateRequest
): Promise<AccountDetail> {
  return requestApiData<AccountDetail>({
    data: request,
    method: 'post',
    url: '/api/accounts',
  })
}

export function getAccount(id: IdString): Promise<AccountDetail> {
  return requestApiData<AccountDetail>({
    method: 'get',
    url: `/api/accounts/${id}`,
  })
}

export function updateAccount(
  id: IdString,
  request: AccountUpdateRequest
): Promise<AccountDetail> {
  return requestApiData<AccountDetail>({
    data: request,
    method: 'put',
    url: `/api/accounts/${id}`,
  })
}

export function deleteAccount(id: IdString): Promise<null> {
  return requestApiData<null>({ method: 'delete', url: `/api/accounts/${id}` })
}

export function archiveAccount(id: IdString): Promise<AccountDetail> {
  return requestApiData<AccountDetail>({
    method: 'post',
    url: `/api/accounts/${id}/archive`,
  })
}

export function restoreAccount(id: IdString): Promise<CollectionRunItem> {
  return requestApiData<CollectionRunItem>({
    method: 'post',
    url: `/api/accounts/${id}/restore`,
  })
}

export function refreshAccount(id: IdString): Promise<AccountDetail> {
  return requestApiData<AccountDetail>({
    method: 'post',
    url: `/api/accounts/${id}/refresh`,
  })
}

export function getAccountStatistics(
  id: IdString,
  params: EntityStatisticsParams
): Promise<StatisticsResponse<AccountStatisticsBreakdown>> {
  return requestApiData<StatisticsResponse<AccountStatisticsBreakdown>>({
    method: 'get',
    params: toSearchParams(params),
    url: `/api/accounts/${id}/stats`,
  })
}
