import type { AccountPage } from '@/features/accounts/types'
import type { CollectionRunItem } from '@/features/sites/types'
import type {
  CustomerStatisticsBreakdown,
  EntityStatisticsParams,
  StatisticsResponse,
} from '@/features/statistics/types'
import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  CustomerCreateRequest,
  CustomerDetail,
  CustomerListParams,
  CustomerPage,
  CustomerUpdateRequest,
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

export function listCustomers(
  params: CustomerListParams
): Promise<CustomerPage> {
  return requestApiData<CustomerPage>({
    method: 'get',
    params: toSearchParams(params),
    url: '/api/customers',
  })
}

const customerOptionPageSize = 100
const customerOptionMaximumPages = 100

export async function listAllCustomers(
  params: Omit<CustomerListParams, 'p' | 'page_size'> = {}
): Promise<CustomerPage> {
  const first = await listCustomers({
    ...params,
    p: 1,
    page_size: customerOptionPageSize,
  })
  const total = BigInt(first.total)
  const remaining: CustomerPage[] = []
  for (
    let page = 2;
    BigInt(page - 1) * BigInt(customerOptionPageSize) < total;
    page += 1
  ) {
    if (page > customerOptionMaximumPages) {
      throw new Error('customer option result exceeds the supported page limit')
    }
    const next = await listCustomers({
      ...params,
      p: page,
      page_size: customerOptionPageSize,
    })
    if (next.total !== first.total || next.items.length === 0) break
    remaining.push(next)
  }
  const items = [first, ...remaining].flatMap((page) => page.items)
  return {
    ...first,
    items: [...new Map(items.map((item) => [item.id, item])).values()],
  }
}

export function createCustomer(
  request: CustomerCreateRequest
): Promise<CustomerDetail> {
  return requestApiData<CustomerDetail>({
    data: request,
    method: 'post',
    url: '/api/customers',
  })
}

export function getCustomer(id: IdString): Promise<CustomerDetail> {
  return requestApiData<CustomerDetail>({
    method: 'get',
    url: `/api/customers/${id}`,
  })
}

export function updateCustomer(
  id: IdString,
  request: CustomerUpdateRequest
): Promise<CustomerDetail> {
  return requestApiData<CustomerDetail>({
    data: request,
    method: 'put',
    url: `/api/customers/${id}`,
  })
}

export function deleteCustomer(id: IdString): Promise<null> {
  return requestApiData<null>({ method: 'delete', url: `/api/customers/${id}` })
}

export function disableCustomer(id: IdString): Promise<CustomerDetail> {
  return requestApiData<CustomerDetail>({
    method: 'post',
    url: `/api/customers/${id}/disable`,
  })
}

export function enableCustomer(id: IdString): Promise<CollectionRunItem> {
  return requestApiData<CollectionRunItem>({
    method: 'post',
    url: `/api/customers/${id}/enable`,
  })
}

export function listCustomerAccounts(
  id: IdString,
  params: { p?: number; page_size?: number }
): Promise<AccountPage> {
  return requestApiData<AccountPage>({
    method: 'get',
    params,
    url: `/api/customers/${id}/accounts`,
  })
}

export function getCustomerStatistics(
  id: IdString,
  params: EntityStatisticsParams
): Promise<StatisticsResponse<CustomerStatisticsBreakdown>> {
  return requestApiData<StatisticsResponse<CustomerStatisticsBreakdown>>({
    method: 'get',
    params: toSearchParams(params),
    url: `/api/customers/${id}/stats`,
  })
}
