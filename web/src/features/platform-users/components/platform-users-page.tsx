import {
  Add01Icon,
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Edit03Icon,
  FilterIcon,
  Key01Icon,
  Refresh01Icon,
  UserAdd01Icon,
  UserRemove01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { fromUnixSeconds } from '@/lib/dayjs'
import { useAuthStore } from '@/stores/auth-store'

import { listPlatformUsers } from '../api'
import { platformUserKeys } from '../query-keys'
import type {
  PlatformUserItem,
  PlatformUserListParams,
  PlatformUserSearch,
} from '../types'
import {
  CreateUserDialog,
  EditUserDialog,
  ResetPasswordDialog,
  ToggleUserDialog,
} from './user-dialogs'

interface PlatformUsersPageProps {
  onSearchChange: (changes: Partial<PlatformUserSearch>) => void
  search: PlatformUserSearch
}

function formatTime(timestamp: number | null): string | null {
  return timestamp == null
    ? null
    : fromUnixSeconds(timestamp).format('YYYY-MM-DD HH:mm:ss')
}

function parseStatusFilter(value: string): 1 | 2 | undefined {
  if (value === '1') return 1
  if (value === '2') return 2
  return undefined
}

function UserFilterFields({
  filter,
  mobile = false,
  onFilterChange,
  onSearchChange,
  search,
}: {
  filter: string
  mobile?: boolean
  onFilterChange: (value: string) => void
  onSearchChange: (changes: Partial<PlatformUserSearch>) => void
  search: PlatformUserSearch
}) {
  const { t } = useTranslation()
  return (
    <>
      <Input
        aria-label={t('Search platform users')}
        className={mobile ? 'w-full' : 'max-w-xs'}
        onChange={(event) => onFilterChange(event.target.value)}
        placeholder={t('Search username or display name')}
        value={filter}
      />
      <Select
        aria-label={t('Filter by role')}
        className={mobile ? 'w-full' : undefined}
        onChange={(event) =>
          onSearchChange({
            page: 1,
            role:
              event.target.value === ''
                ? undefined
                : (event.target.value as 'admin' | 'viewer'),
          })
        }
        value={search.role ?? ''}
      >
        <option value=''>{t('All roles')}</option>
        <option value='admin'>{t('Administrator')}</option>
        <option value='viewer'>{t('Viewer')}</option>
      </Select>
      <Select
        aria-label={t('Filter by status')}
        className={mobile ? 'w-full' : undefined}
        onChange={(event) =>
          onSearchChange({
            page: 1,
            status: parseStatusFilter(event.target.value),
          })
        }
        value={search.status?.toString() ?? ''}
      >
        <option value=''>{t('All statuses')}</option>
        <option value='1'>{t('Enabled')}</option>
        <option value='2'>{t('Disabled')}</option>
      </Select>
    </>
  )
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
  onSearchChange,
  search,
}: PlatformUsersPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.user)
  const isAdmin = currentUser?.role === 'admin'
  const [filter, setFilter] = useState(search.filter)
  const [mobileFiltersOpen, setMobileFiltersOpen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [editUser, setEditUser] = useState<PlatformUserItem | null>(null)
  const [resetUser, setResetUser] = useState<PlatformUserItem | null>(null)
  const [toggleState, setToggleState] = useState<{
    action: 'enable' | 'disable'
    user: PlatformUserItem
  } | null>(null)

  useEffect(() => setFilter(search.filter), [search.filter])

  const params = useMemo<PlatformUserListParams>(
    () => ({
      keyword: search.filter || undefined,
      p: search.page,
      page_size: search.pageSize,
      role: search.role,
      sort_by: 'created_at',
      sort_order: 'desc',
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
  const submitFilter = (event: FormEvent) => {
    event.preventDefault()
    onSearchChange({ filter: filter.trim(), page: 1 })
    setMobileFiltersOpen(false)
  }
  const enabledAdminTotal = enabledAdminQuery.data?.total ?? 0
  const pageData = usersQuery.data
  const totalPages = pageData
    ? Math.max(1, Math.ceil(pageData.total / pageData.page_size))
    : 1
  const initialLoading = usersQuery.isPending && !pageData
  const initialError = usersQuery.isError && !pageData
  const empty = pageData?.items.length === 0
  const hasRows = (pageData?.items.length ?? 0) > 0

  return (
    <SectionPageLayout
      actions={
        isAdmin ? (
          <Button onClick={() => setCreateOpen(true)}>
            <HugeiconsIcon icon={Add01Icon} strokeWidth={2} />
            {t('Create user')}
          </Button>
        ) : undefined
      }
      description={t('Manage platform access and roles')}
      title={t('Platform users')}
    >
      <div className='grid gap-4'>
        <div className='flex items-center gap-2 sm:hidden'>
          <Button
            onClick={() => setMobileFiltersOpen(true)}
            type='button'
            variant='outline'
          >
            <HugeiconsIcon icon={FilterIcon} strokeWidth={2} />
            {t('Filter users')}
          </Button>
          <Button
            aria-label={t('Refresh')}
            disabled={usersQuery.isFetching}
            onClick={() => void usersQuery.refetch()}
            size='icon'
            title={t('Refresh')}
            type='button'
            variant='ghost'
          >
            {usersQuery.isFetching ? (
              <Spinner />
            ) : (
              <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
            )}
          </Button>
        </div>
        <form
          className='hidden gap-2 sm:flex sm:items-center'
          onSubmit={submitFilter}
        >
          <UserFilterFields
            filter={filter}
            onFilterChange={setFilter}
            onSearchChange={onSearchChange}
            search={search}
          />
          <Button type='submit' variant='outline'>
            {t('Search')}
          </Button>
          <Button
            aria-label={t('Refresh')}
            disabled={usersQuery.isFetching}
            onClick={() => void usersQuery.refetch()}
            size='icon'
            title={t('Refresh')}
            type='button'
            variant='ghost'
          >
            {usersQuery.isFetching ? (
              <Spinner />
            ) : (
              <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
            )}
          </Button>
        </form>

        <Sheet onOpenChange={setMobileFiltersOpen} open={mobileFiltersOpen}>
          <SheetContent className='sm:hidden'>
            <SheetHeader>
              <SheetTitle>{t('Filter platform users')}</SheetTitle>
              <SheetDescription>
                {t('Filter the platform user list')}
              </SheetDescription>
            </SheetHeader>
            <form className='grid gap-4' onSubmit={submitFilter}>
              <UserFilterFields
                filter={filter}
                mobile
                onFilterChange={setFilter}
                onSearchChange={onSearchChange}
                search={search}
              />
              <Button type='submit'>{t('Apply filters')}</Button>
            </form>
          </SheetContent>
        </Sheet>

        {initialLoading && (
          <div
            aria-hidden='true'
            className='border-border bg-card h-64 animate-pulse rounded-lg border'
          />
        )}
        {initialError && (
          <section className='border-destructive/30 bg-destructive/5 rounded-lg border p-5'>
            <h2 className='font-medium'>
              {t('Unable to load platform users')}
            </h2>
            <Button
              className='mt-3'
              onClick={() => void usersQuery.refetch()}
              variant='outline'
            >
              {t('Retry')}
            </Button>
          </section>
        )}
        {empty && (
          <section className='border-border bg-card rounded-lg border p-8 text-center'>
            <h2 className='font-medium'>{t('No platform users found')}</h2>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('Adjust the filters and try again')}
            </p>
          </section>
        )}
        {hasRows && (
          <>
            <div className='hidden overflow-hidden rounded-lg border xl:block'>
              <table className='w-full border-collapse text-sm'>
                <thead className='bg-muted/70 text-left'>
                  <tr>
                    <th className='px-3 py-2.5 font-medium'>{t('Username')}</th>
                    <th className='px-3 py-2.5 font-medium'>
                      {t('Display name')}
                    </th>
                    <th className='px-3 py-2.5 font-medium'>{t('Role')}</th>
                    <th className='px-3 py-2.5 font-medium'>{t('Status')}</th>
                    <th className='px-3 py-2.5 font-medium'>
                      {t('Password change')}
                    </th>
                    <th className='px-3 py-2.5 font-medium'>
                      {t('Last signed in')}
                    </th>
                    <th
                      aria-sort='descending'
                      className='px-3 py-2.5 font-medium'
                    >
                      {t('Created at')}
                    </th>
                    {isAdmin && (
                      <th className='px-3 py-2.5 text-right font-medium'>
                        {t('Actions')}
                      </th>
                    )}
                  </tr>
                </thead>
                <tbody>
                  {pageData?.items.map((user) => (
                    <UserTableRow
                      currentUserId={currentUser?.id}
                      enabledAdminTotal={enabledAdminTotal}
                      isAdmin={isAdmin}
                      key={user.id}
                      onEdit={setEditUser}
                      onReset={setResetUser}
                      onToggle={(target, action) =>
                        setToggleState({ action, user: target })
                      }
                      user={user}
                    />
                  ))}
                </tbody>
              </table>
            </div>
            <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:hidden'>
              {pageData?.items.map((user) => (
                <UserCard
                  currentUserId={currentUser?.id}
                  enabledAdminTotal={enabledAdminTotal}
                  isAdmin={isAdmin}
                  key={user.id}
                  onEdit={setEditUser}
                  onReset={setResetUser}
                  onToggle={(target, action) =>
                    setToggleState({ action, user: target })
                  }
                  user={user}
                />
              ))}
            </div>
          </>
        )}

        {pageData && pageData.total > 0 && (
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <p className='text-muted-foreground text-sm'>
              {t('{{total}} users', { total: pageData.total })}
            </p>
            <div className='flex items-center gap-2'>
              <Button
                aria-label={t('Previous page')}
                disabled={search.page <= 1}
                onClick={() => onSearchChange({ page: search.page - 1 })}
                size='icon'
                title={t('Previous page')}
                variant='outline'
              >
                <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
              </Button>
              <span className='min-w-20 text-center text-sm'>
                {t('{{page}} / {{pages}}', {
                  page: search.page,
                  pages: totalPages,
                })}
              </span>
              <Button
                aria-label={t('Next page')}
                disabled={search.page >= totalPages}
                onClick={() => onSearchChange({ page: search.page + 1 })}
                size='icon'
                title={t('Next page')}
                variant='outline'
              >
                <HugeiconsIcon icon={ArrowRight01Icon} strokeWidth={2} />
              </Button>
            </div>
          </div>
        )}
      </div>

      <CreateUserDialog
        onOpenChange={setCreateOpen}
        onSaved={invalidateUsers}
        open={createOpen}
      />
      <EditUserDialog
        isLastEnabledAdmin={
          editUser?.role === 'admin' &&
          editUser.status === 1 &&
          enabledAdminTotal <= 1
        }
        onOpenChange={(open) => !open && setEditUser(null)}
        onSaved={invalidateUsers}
        open={editUser != null}
        user={editUser}
      />
      <ResetPasswordDialog
        onOpenChange={(open) => !open && setResetUser(null)}
        onSaved={invalidateUsers}
        open={resetUser != null}
        user={resetUser}
      />
      <ToggleUserDialog
        action={toggleState?.action ?? 'disable'}
        onOpenChange={(open) => !open && setToggleState(null)}
        onSaved={invalidateUsers}
        open={toggleState != null}
        user={toggleState?.user ?? null}
      />
    </SectionPageLayout>
  )
}

interface UserActionsProps {
  currentUserId: string | undefined
  enabledAdminTotal: number
  isAdmin: boolean
  onEdit: (user: PlatformUserItem) => void
  onReset: (user: PlatformUserItem) => void
  onToggle: (user: PlatformUserItem, action: 'enable' | 'disable') => void
  user: PlatformUserItem
}

function UserActions({
  currentUserId,
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
    user.role === 'admin' && user.status === 1 && enabledAdminTotal <= 1
  const toggleDisabled = isCurrentUser || isLastEnabledAdmin
  const toggleLabel = user.status === 1 ? 'Disable user' : 'Enable user'
  let toggleTitle = t(dynamicI18nKey('platformUser', toggleLabel))
  if (isCurrentUser) toggleTitle = t('You cannot disable your own account')
  else if (isLastEnabledAdmin) {
    toggleTitle = t('The last enabled administrator cannot be disabled')
  }

  return (
    <div className='flex justify-end gap-1'>
      <Button
        aria-label={t('Edit user')}
        onClick={() => onEdit(user)}
        size='icon'
        title={t('Edit user')}
        variant='ghost'
      >
        <HugeiconsIcon icon={Edit03Icon} strokeWidth={2} />
      </Button>
      <Button
        aria-label={t('Reset password')}
        disabled={isCurrentUser}
        onClick={() => onReset(user)}
        size='icon'
        title={
          isCurrentUser
            ? t('Use Change password for your own account')
            : t('Reset password')
        }
        variant='ghost'
      >
        <HugeiconsIcon icon={Key01Icon} strokeWidth={2} />
      </Button>
      <Button
        aria-label={t(dynamicI18nKey('platformUser', toggleLabel))}
        disabled={toggleDisabled}
        onClick={() => onToggle(user, user.status === 1 ? 'disable' : 'enable')}
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

function UserTableRow(props: UserActionsProps) {
  const { t } = useTranslation()
  const { user } = props
  const lastLogin = formatTime(user.last_login_at)
  const createdAt = formatTime(user.created_at)
  return (
    <tr className='border-t'>
      <td className='px-3 py-3 font-medium'>{user.username}</td>
      <td className='px-3 py-3'>{user.display_name}</td>
      <td className='px-3 py-3'>
        {t(
          dynamicI18nKey(
            'platformUser',
            user.role === 'admin' ? 'Administrator' : 'Viewer'
          )
        )}
      </td>
      <td className='px-3 py-3'>
        <UserStatusBadge user={user} />
      </td>
      <td className='px-3 py-3'>
        {user.must_change_password ? t('Required') : t('Not required')}
      </td>
      <td className='px-3 py-3' title={lastLogin ?? undefined}>
        {lastLogin ?? t('Never')}
      </td>
      <td className='px-3 py-3' title={createdAt ?? undefined}>
        {createdAt}
      </td>
      {props.isAdmin && (
        <td className='px-2 py-1'>
          <UserActions {...props} />
        </td>
      )}
    </tr>
  )
}

function UserCard(props: UserActionsProps) {
  const { t } = useTranslation()
  const { user } = props
  const lastLogin = formatTime(user.last_login_at)
  const createdAt = formatTime(user.created_at)
  return (
    <article className='border-border bg-card rounded-lg border p-4'>
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0'>
          <h2 className='truncate font-medium'>{user.display_name}</h2>
          <p className='text-muted-foreground truncate text-sm'>
            {user.username}
          </p>
        </div>
        <UserStatusBadge user={user} />
      </div>
      <dl className='mt-4 grid grid-cols-2 gap-3 text-sm'>
        <div>
          <dt className='text-muted-foreground'>{t('Role')}</dt>
          <dd>
            {t(
              dynamicI18nKey(
                'platformUser',
                user.role === 'admin' ? 'Administrator' : 'Viewer'
              )
            )}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Password change')}</dt>
          <dd>
            {user.must_change_password ? t('Required') : t('Not required')}
          </dd>
        </div>
        <div className='col-span-2'>
          <dt className='text-muted-foreground'>{t('Last signed in')}</dt>
          <dd>{lastLogin ?? t('Never')}</dd>
        </div>
        <div className='col-span-2'>
          <dt className='text-muted-foreground'>{t('Created at')}</dt>
          <dd>{createdAt}</dd>
        </div>
      </dl>
      {props.isAdmin && (
        <div className='mt-3 border-t pt-2'>
          <UserActions {...props} />
        </div>
      )}
    </article>
  )
}
