import { Menu } from '@base-ui/react/menu'
import { Key01Icon, Logout01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useRouter } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import type { LoginUser } from '@/features/auth/types'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'

import { Avatar, AvatarFallback } from '../ui/avatar'
import { buttonVariants } from '../ui/button'
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
        <Menu.Root>
          <Menu.Trigger
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
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Positioner align='end' sideOffset={8}>
              <Menu.Popup className='bg-popover text-popover-foreground ring-foreground/10 z-50 w-56 rounded-lg p-1 shadow-md ring-1 outline-none'>
                <div className='flex items-center gap-2 px-1.5 py-1.5'>
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
                </div>
                <div className='bg-border -mx-1 my-1 h-px' />
                {!user.must_change_password && (
                  <Menu.Item
                    className='data-highlighted:bg-accent data-highlighted:text-accent-foreground flex w-full items-center gap-2 rounded-md px-1.5 py-1.5 text-sm outline-none'
                    onClick={() =>
                      void router.navigate({ to: '/change-password' })
                    }
                  >
                    <HugeiconsIcon icon={Key01Icon} size={16} strokeWidth={2} />
                    {t('Change password')}
                  </Menu.Item>
                )}
                <div className='bg-border -mx-1 my-1 h-px' />
                <Menu.Item
                  className='data-highlighted:bg-destructive/10 text-destructive flex w-full items-center gap-2 rounded-md px-1.5 py-1.5 text-sm outline-none'
                  disabled={isLoggingOut}
                  onClick={onLogout}
                >
                  <HugeiconsIcon
                    icon={Logout01Icon}
                    size={16}
                    strokeWidth={2}
                  />
                  {t('Sign out')}
                </Menu.Item>
              </Menu.Popup>
            </Menu.Positioner>
          </Menu.Portal>
        </Menu.Root>
      </div>
    </Header>
  )
}
