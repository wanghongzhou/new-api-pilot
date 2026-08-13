import {
  Add01Icon,
  Edit03Icon,
  Key01Icon,
  UserAdd01Icon,
  UserRemove01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PageFooterPortal } from '@/components/layout/page-footer'
import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/data-table'
import { DataTablePagination } from '@/components/ui/data-table-pagination'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import type { MetricString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import { useAuthStore } from '@/stores/auth-store'

import { listPlatformUsers } from '../api'
import { platformUserKeys } from '../query-keys'
import type {
  PlatformUserItem,
  PlatformUserListParams,
  PlatformUserSearch,
} from '../types'
import { PlatformUserFilters } from './platform-user-filters'
import {
  CreateUserDialog,
  EditUserDialog,
  ResetPasswordDialog,
  ToggleUserDialog,
} from './user-dialogs'

interface PlatformUsersPageProps {
  onPageReplace: (page: number) => void
  onSearchChange: (changes: Partial<PlatformUserSearch>) => void
  search: PlatformUserSearch
}

function formatTime(timestamp: number | null): string | null {
  return timestamp == null
    ? null
    : fromUnixSeconds(timestamp).format('YYYY-MM-DD HH:mm:ss')
}

function UserStatusBadge({ user }: { user: PlatformUserItem }) {
  const { t } = useTranslation()
  return (
    <Badge variant={user.status === 1 ? 'success' : 'neutral'}>
      {t(
        dynamicI18nKey(
          'platformUser',
          user.status === 1 ? 'Enabled' : 'Disabled'
        )
      )}
    </Badge>
  )
}

export function PlatformUsersPage({
  onPageReplace,
  onSearchChange,
  search,
}: PlatformUsersPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.user)
  const isAdmin = currentUser?.role === 'admin'
  const [createOpen, setCreateOpen] = useState(false)
  const [editUser, setEditUser] = useState<PlatformUserItem | null>(null)
  const [resetUser, setResetUser] = useState<PlatformUserItem | null>(null)
  const [toggleState, setToggleState] = useState<{
    action: 'enable' | 'disable'
    user: PlatformUserItem
  } | null>(null)
  const toggleTriggerRef = useRef<HTMLButtonElement | null>(null)

  const params = useMemo<PlatformUserListParams>(
    () => ({
      keyword: search.filter || undefined,
      p: search.page,
      page_size: search.pageSize,
      role: search.role,
      sort_by: search.sort ?? 'created_at',
      sort_order: search.order ?? 'desc',
      status: search.status,
    }),
    [search]
  )
  const usersQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => listPlatformUsers(params),
    queryKey: platformUserKeys.list(params),
    staleTime: 30_000,
  })
  const enabledAdminQuery = useQuery({
    enabled: isAdmin,
    queryFn: () =>
      listPlatformUsers({
        p: 1,
        page_size: 1,
        role: 'admin',
        status: 1,
      }),
    queryKey: platformUserKeys.enabledAdminCount(),
    staleTime: 30_000,
  })

  const invalidateUsers = () => {
    void queryClient.invalidateQueries({ queryKey: platformUserKeys.all })
  }
  const enabledAdminTotal = enabledAdminQuery.data?.total ?? null
  const pageData = usersQuery.data
  const listStale = usersQuery.isError && pageData != null
  let actionsDisabledReason: string | undefined
  if (listStale) {
    actionsDisabledReason = t('Platform user data may be outdated')
  } else if (isAdmin && enabledAdminTotal == null) {
    actionsDisabledReason = t('Unable to verify administrator safeguards')
  }
  const initialLoading = usersQuery.isPending && !pageData
  useEffect(() => {
    if (
      !pageData ||
      pageData.total === '0' ||
      usersQuery.isFetching ||
      usersQuery.isPlaceholderData
    ) {
      return
    }
    const firstOffset = BigInt(search.page - 1) * BigInt(pageData.page_size)
    if (search.page > 1 && firstOffset >= BigInt(pageData.total)) {
      onPageReplace(search.page - 1)
    }
  }, [
    onPageReplace,
    pageData,
    search.page,
    usersQuery.isFetching,
    usersQuery.isPlaceholderData,
  ])
  const updateSorting = (
    updater: SortingState | ((old: SortingState) => SortingState)
  ) => {
    const current = [
      {
        desc: (search.order ?? 'desc') === 'desc',
        id: search.sort ?? 'created_at',
      },
    ]
    const next = typeof updater === 'function' ? updater(current) : updater
    const first = next[0]
    if (!first) {
      onSearchChange({ order: 'desc', page: 1, sort: 'created_at' })
      return
    }
    onSearchChange({
      order: first.desc ? 'desc' : 'asc',
      page: 1,
      sort: first.id as PlatformUserSearch['sort'],
    })
  }
  const columns = useMemo<ColumnDef<PlatformUserItem, unknown>[]>(
    () => [
      {
        accessorKey: 'username',
        cell: ({ row }) => (
          <span className='font-medium'>{row.original.username}</span>
        ),
        header: t('Username'),
      },
      {
        accessorKey: 'display_name',
        cell: ({ row }) => (
          <span className='block min-w-0 break-words'>
            {row.original.display_name}
          </span>
        ),
        enableSorting: false,
        header: t('Display name'),
      },
      {
        cell: ({ row }) =>
          t(
            dynamicI18nKey(
              'platformUser',
              row.original.role === 'admin' ? 'Administrator' : 'Viewer'
            )
          ),
        header: t('Role'),
        id: 'role',
      },
      {
        accessorKey: 'status',
        cell: ({ row }) => <UserStatusBadge user={row.original} />,
        enableSorting: true,
        header: t('Status'),
      },
      {
        cell: ({ row }) =>
          row.original.must_change_password ? t('Required') : t('Not required'),
        header: t('Password change'),
        id: 'passwordChange',
      },
      {
        accessorKey: 'last_login_at',
        cell: ({ row }) => formatTime(row.original.last_login_at) ?? t('Never'),
        enableSorting: true,
        header: t('Last signed in'),
      },
      {
        accessorKey: 'created_at',
        cell: ({ row }) => formatTime(row.original.created_at),
        enableSorting: true,
        header: t('Created at'),
      },
      ...(isAdmin
        ? [
            {
              cell: ({ row }: { row: { original: PlatformUserItem } }) => (
                <UserActions
                  currentUserId={currentUser?.id}
                  disabledReason={actionsDisabledReason}
                  enabledAdminTotal={enabledAdminTotal}
                  isAdmin={isAdmin}
                  onEdit={setEditUser}
                  onReset={setResetUser}
                  onToggle={(target, action, trigger) => {
                    toggleTriggerRef.current = trigger
                    setToggleState({ action, user: target })
                  }}
                  user={row.original}
                />
              ),
              header: t('Actions'),
              id: 'actions',
            },
          ]
        : []),
    ],
    [actionsDisabledReason, currentUser?.id, enabledAdminTotal, isAdmin, t]
  )

  return (
    <SectionPageLayout
      actions={
        isAdmin ? (
          <Button
            disabled={Boolean(actionsDisabledReason)}
            onClick={() => setCreateOpen(true)}
            title={actionsDisabledReason}
          >
            <HugeiconsIcon icon={Add01Icon} strokeWidth={2} />
            {t('Create user')}
          </Button>
        ) : undefined
      }
      description={t('Manage platform access and roles')}
      fixedContent
      title={t('Platform users')}
    >
      <div className='flex h-full min-h-0 min-w-0 flex-col gap-5'>
        <PlatformUserFilters
          onApply={(filters) => onSearchChange({ ...filters, page: 1 })}
          value={{
            filter: search.filter,
            role: search.role,
            status: search.status,
          }}
        />

        {isAdmin && enabledAdminQuery.isError && (
          <div
            className='border-destructive/35 bg-destructive/5 text-destructive flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3 py-2 text-sm'
            role='alert'
          >
            <span>{t('Unable to verify administrator safeguards')}</span>
            <Button
              onClick={() => void enabledAdminQuery.refetch()}
              size='sm'
              type='button'
              variant='outline'
            >
              {t('common.retry')}
            </Button>
          </div>
        )}
        {listStale && (
          <div
            className='border-warning/40 bg-warning/10 text-warning-foreground flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3 py-2 text-sm'
            role='alert'
          >
            <span>{t('Platform user data may be outdated')}</span>
            <Button
              onClick={() => void usersQuery.refetch()}
              size='sm'
              type='button'
              variant='outline'
            >
              {t('common.retry')}
            </Button>
          </div>
        )}

        <div className='flex min-h-0 flex-1 flex-col'>
          <DataTable
            ariaLabel={t('Platform users')}
            columns={columns}
            data={pageData?.items ?? []}
            emptyDescription={t('Adjust the filters and try again')}
            emptyTitle={t('No platform users found')}
            error={usersQuery.isError}
            fetching={usersQuery.isFetching && !initialLoading}
            fillAvailableHeight
            loading={initialLoading}
            onRetry={() => void usersQuery.refetch()}
            onSortingChange={updateSorting}
            preserveHeaderWhenEmpty
            renderMobileCard={(user) => (
              <UserCard
                currentUserId={currentUser?.id}
                disabledReason={actionsDisabledReason}
                enabledAdminTotal={enabledAdminTotal}
                isAdmin={isAdmin}
                onEdit={setEditUser}
                onReset={setResetUser}
                onToggle={(target, action, trigger) => {
                  toggleTriggerRef.current = trigger
                  setToggleState({ action, user: target })
                }}
                user={user}
              />
            )}
            sorting={[
              {
                desc: (search.order ?? 'desc') === 'desc',
                id: search.sort ?? 'created_at',
              },
            ]}
          />
        </div>
      </div>

      <PageFooterPortal>
        <DataTablePagination
          onPageChange={(page) => onSearchChange({ page })}
          onPageSizeChange={(pageSize) => onSearchChange({ page: 1, pageSize })}
          page={search.page}
          pageSize={pageData?.page_size ?? search.pageSize}
          total={pageData?.total ?? 0}
        />
      </PageFooterPortal>

      <CreateUserDialog
        disabledReason={actionsDisabledReason}
        onOpenChange={setCreateOpen}
        onSaved={invalidateUsers}
        open={createOpen}
      />
      <EditUserDialog
        disabledReason={actionsDisabledReason}
        isLastEnabledAdmin={
          editUser?.role === 'admin' &&
          editUser.status === 1 &&
          enabledAdminTotal != null &&
          BigInt(enabledAdminTotal) <= 1n
        }
        onOpenChange={(open) => !open && setEditUser(null)}
        onSaved={invalidateUsers}
        open={editUser != null}
        user={editUser}
      />
      <ResetPasswordDialog
        disabledReason={actionsDisabledReason}
        onOpenChange={(open) => !open && setResetUser(null)}
        onSaved={invalidateUsers}
        open={resetUser != null}
        user={resetUser}
      />
      <ToggleUserDialog
        action={toggleState?.action ?? 'disable'}
        disabledReason={actionsDisabledReason}
        onOpenChange={(open) => {
          if (open) return
          const trigger = toggleTriggerRef.current
          setToggleState(null)
          requestAnimationFrame(() => trigger?.focus())
        }}
        onSaved={invalidateUsers}
        open={toggleState != null}
        user={toggleState?.user ?? null}
      />
    </SectionPageLayout>
  )
}

interface UserActionsProps {
  currentUserId: string | undefined
  disabledReason?: string
  enabledAdminTotal: MetricString | null
  isAdmin: boolean
  onEdit: (user: PlatformUserItem) => void
  onReset: (user: PlatformUserItem) => void
  onToggle: (
    user: PlatformUserItem,
    action: 'enable' | 'disable',
    trigger: HTMLButtonElement
  ) => void
  user: PlatformUserItem
}

function UserActions({
  currentUserId,
  disabledReason,
  enabledAdminTotal,
  isAdmin,
  onEdit,
  onReset,
  onToggle,
  user,
}: UserActionsProps) {
  const { t } = useTranslation()
  if (!isAdmin) return null

  const isCurrentUser = user.id === currentUserId
  const isLastEnabledAdmin =
    user.role === 'admin' &&
    user.status === 1 &&
    enabledAdminTotal != null &&
    BigInt(enabledAdminTotal) <= 1n
  const adminSafeguardUnknown =
    user.role === 'admin' && user.status === 1 && enabledAdminTotal == null
  const toggleDisabled =
    Boolean(disabledReason) ||
    isCurrentUser ||
    isLastEnabledAdmin ||
    adminSafeguardUnknown
  const toggleLabel = user.status === 1 ? 'Disable user' : 'Enable user'
  let toggleTitle =
    disabledReason ?? t(dynamicI18nKey('platformUser', toggleLabel))
  const editTitle =
    disabledReason ??
    (adminSafeguardUnknown
      ? t('Unable to verify administrator safeguards')
      : t('Edit user'))
  if (disabledReason) toggleTitle = disabledReason
  else if (isCurrentUser) toggleTitle = t('You cannot disable your own account')
  else if (isLastEnabledAdmin) {
    toggleTitle = t('The last enabled administrator cannot be disabled')
  } else if (adminSafeguardUnknown) {
    toggleTitle = t('Unable to verify administrator safeguards')
  }

  return (
    <div className='flex justify-end gap-1'>
      <Button
        aria-label={t('Edit user')}
        disabled={Boolean(disabledReason) || adminSafeguardUnknown}
        onClick={() => onEdit(user)}
        className='min-h-10 min-w-10'
        size='icon'
        title={editTitle}
        variant='ghost'
      >
        <HugeiconsIcon icon={Edit03Icon} strokeWidth={2} />
      </Button>
      <Button
        aria-label={t('Reset password')}
        disabled={Boolean(disabledReason) || isCurrentUser}
        onClick={() => onReset(user)}
        className='min-h-10 min-w-10'
        size='icon'
        title={
          disabledReason ??
          (isCurrentUser
            ? t('Use Change password for your own account')
            : t('Reset password'))
        }
        variant='ghost'
      >
        <HugeiconsIcon icon={Key01Icon} strokeWidth={2} />
      </Button>
      <Button
        aria-label={t(dynamicI18nKey('platformUser', toggleLabel))}
        disabled={toggleDisabled}
        onClick={(event) =>
          onToggle(
            user,
            user.status === 1 ? 'disable' : 'enable',
            event.currentTarget
          )
        }
        className='min-h-10 min-w-10'
        size='icon'
        title={toggleTitle}
        variant={user.status === 1 ? 'ghost' : 'outline'}
      >
        <HugeiconsIcon
          icon={user.status === 1 ? UserRemove01Icon : UserAdd01Icon}
          strokeWidth={2}
        />
      </Button>
    </div>
  )
}

function UserCard(props: UserActionsProps) {
  const { t } = useTranslation()
  const { user } = props
  const lastLogin = formatTime(user.last_login_at)
  const createdAt = formatTime(user.created_at)
  return (
    <article className='bg-card text-card-foreground ring-foreground/10 rounded-xl p-4 ring-1'>
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0'>
          <h2 className='font-medium break-words'>{user.display_name}</h2>
          <p className='text-muted-foreground truncate text-sm'>
            {user.username}
          </p>
        </div>
        <UserStatusBadge user={user} />
      </div>
      <div className='mt-4 grid grid-cols-2 gap-3 text-sm'>
        <dl>
          <dt className='text-muted-foreground'>{t('Role')}</dt>
          <dd>
            {t(
              dynamicI18nKey(
                'platformUser',
                user.role === 'admin' ? 'Administrator' : 'Viewer'
              )
            )}
          </dd>
        </dl>
        <dl>
          <dt className='text-muted-foreground'>{t('Password change')}</dt>
          <dd>
            {user.must_change_password ? t('Required') : t('Not required')}
          </dd>
        </dl>
        <dl className='col-span-2'>
          <dt className='text-muted-foreground'>{t('Last signed in')}</dt>
          <dd>{lastLogin ?? t('Never')}</dd>
        </dl>
        <dl className='col-span-2'>
          <dt className='text-muted-foreground'>{t('Created at')}</dt>
          <dd>{createdAt}</dd>
        </dl>
      </div>
      {props.isAdmin && (
        <div className='mt-3 border-t pt-2'>
          <UserActions {...props} />
        </div>
      )}
    </article>
  )
}
