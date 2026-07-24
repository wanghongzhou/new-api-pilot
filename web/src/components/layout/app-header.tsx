import { Key01Icon, Logout01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useRouter } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import type { LoginUser } from '@/features/auth/types'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'

import { Avatar, AvatarFallback } from '../ui/avatar'
import { buttonVariants } from '../ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '../ui/dropdown-menu'
import { Brand } from './brand'
import { Header } from './header'
import { ThemeSettingsDrawer } from './theme-settings-drawer'

export function AppHeader({
  isLoggingOut,
  onLogout,
  showMenu = true,
  user,
}: {
  isLoggingOut: boolean
  onLogout: () => void
  showMenu?: boolean
  user: LoginUser
}) {
  const { t } = useTranslation()
  const router = useRouter()
  const avatarName = user.username || user.display_name
  const avatarFallback = getUserAvatarFallback(avatarName)
  const avatarFallbackStyle = useMemo(
    () => getUserAvatarStyle(avatarName),
    [avatarName]
  )
  const roleLabel =
    user.role === 'admin' ? t('user.role.admin') : t('user.role.viewer')

  return (
    <Header showTrigger={showMenu}>
      <Brand variant='inline' />
      <div className='ms-auto flex items-center gap-1 sm:gap-2'>
        <ThemeSettingsDrawer />
        <DropdownMenu>
          <DropdownMenuTrigger
            aria-label={user.display_name}
            className={buttonVariants({
              className: 'relative size-6 p-0',
              variant: 'ghost',
            })}
            title={user.display_name}
          >
            <Avatar className='size-6'>
              <AvatarFallback
                className='text-[11px] font-semibold text-white'
                style={avatarFallbackStyle}
              >
                {avatarFallback}
              </AvatarFallback>
            </Avatar>
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' className='w-56' sideOffset={8}>
            <DropdownMenuLabel className='flex items-center gap-2 px-1.5 py-1.5 font-normal'>
              <Avatar className='size-8'>
                <AvatarFallback
                  className='text-xs font-semibold text-white'
                  style={avatarFallbackStyle}
                >
                  {avatarFallback}
                </AvatarFallback>
              </Avatar>
              <div className='flex min-w-0 flex-1 flex-col gap-0.5 overflow-hidden'>
                <p className='text-foreground truncate text-sm font-medium'>
                  {user.display_name}
                </p>
                <span className='text-muted-foreground truncate text-xs'>
                  {roleLabel}
                </span>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            {!user.must_change_password && (
              <DropdownMenuItem
                onClick={() => void router.navigate({ to: '/change-password' })}
              >
                <HugeiconsIcon icon={Key01Icon} size={16} strokeWidth={2} />
                {t('Change password')}
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem
              disabled={isLoggingOut}
              onClick={onLogout}
              variant='destructive'
            >
              <HugeiconsIcon icon={Logout01Icon} size={16} strokeWidth={2} />
              {t('Sign out')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </Header>
  )
}
