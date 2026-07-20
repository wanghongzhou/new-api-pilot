import type { PlatformUserListParams } from './types'

export const platformUserKeys = {
  all: ['platform-users'] as const,
  enabledAdminCount: () => ['platform-users', 'enabled-admin-count'] as const,
  list: (params: PlatformUserListParams) =>
    ['platform-users', 'list', params] as const,
  lists: () => ['platform-users', 'list'] as const,
}
