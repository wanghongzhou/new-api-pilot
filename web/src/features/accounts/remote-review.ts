import type { RemoteUserItem, RemoteUserPage } from './types'

const reviewedRemoteUserFields = [
  'id',
  'username',
  'display_name',
  'role',
  'status',
  'group',
  'quota',
  'used_quota',
  'request_count',
  'created_at',
] as const satisfies readonly (keyof RemoteUserItem)[]

export function findExactRemoteUser(
  page: RemoteUserPage,
  remoteUserId: string
): RemoteUserItem | null {
  const matches = page.items.filter((item) => item.id === remoteUserId)
  return matches.length === 1 ? (matches[0] ?? null) : null
}

export function remoteUserChanged(
  selected: RemoteUserItem,
  reviewed: RemoteUserItem
): boolean {
  return reviewedRemoteUserFields.some(
    (field) => selected[field] !== reviewed[field]
  )
}
