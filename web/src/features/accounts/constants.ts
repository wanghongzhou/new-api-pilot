import type {
  AccountManagedStatus,
  AccountRemoteState,
  RemoteUserStatusFilter,
} from './types'

export const accountRemoteStates = [
  'normal',
  'missing',
  'identity_mismatch',
] as const satisfies readonly AccountRemoteState[]

export const accountManagedStatuses = [
  'active',
  'archived',
] as const satisfies readonly AccountManagedStatus[]

export const remoteUserStatusFilters = [
  '1',
  '2',
] as const satisfies readonly RemoteUserStatusFilter[]
