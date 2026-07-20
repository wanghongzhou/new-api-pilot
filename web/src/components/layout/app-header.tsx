import {
  FileExportIcon,
  Key01Icon,
  Logout01Icon,
  Menu02Icon,
  Moon02Icon,
  Sun01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { useTheme } from '@/context/theme-provider'
import type { LoginUser } from '@/features/auth/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import { Button, buttonVariants } from '../ui/button'
import { Brand } from './brand'

export function AppHeader({
  isLoggingOut,
  onLogout,
  onOpenMenu,
  showMenu = true,
  user,
}: {
  isLoggingOut: boolean
  onLogout: () => void
  onOpenMenu: () => void
  showMenu?: boolean
  user: LoginUser
}) {
  const { t } = useTranslation()
  const { resolvedTheme, setPreference } = useTheme()
  const nextTheme = resolvedTheme === 'dark' ? 'light' : 'dark'
  const themeLabel =
    nextTheme === 'dark' ? 'Switch to dark theme' : 'Switch to light theme'

  return (
    <header className='bg-background flex h-14 shrink-0 items-center gap-2 border-b px-2 sm:px-4'>
      {showMenu && (
        <Button
          aria-label={t('Open navigation')}
          className='lg:hidden'
          onClick={onOpenMenu}
          size='icon'
          title={t('Open navigation')}
          variant='ghost'
        >
          <HugeiconsIcon icon={Menu02Icon} strokeWidth={2} />
        </Button>
      )}
      <div className='lg:hidden'>
        <Brand compact />
      </div>
      <div className='flex-1' />
      <Link
        aria-label={t('exports.open')}
        className={buttonVariants({ size: 'icon', variant: 'ghost' })}
        title={t('exports.open')}
        to='/exports'
      >
        <HugeiconsIcon icon={FileExportIcon} strokeWidth={2} />
      </Link>
      {!user.must_change_password && (
        <Link
          aria-label={t('Change password')}
          className={buttonVariants({ size: 'icon', variant: 'ghost' })}
          title={t('Change password')}
          to='/change-password'
        >
          <HugeiconsIcon icon={Key01Icon} strokeWidth={2} />
        </Link>
      )}
      <Button
        aria-label={t(dynamicI18nKey('layout', themeLabel))}
        onClick={() => setPreference(nextTheme)}
        size='icon'
        title={t(dynamicI18nKey('layout', themeLabel))}
        variant='ghost'
      >
        <HugeiconsIcon
          icon={resolvedTheme === 'dark' ? Sun01Icon : Moon02Icon}
          strokeWidth={2}
        />
      </Button>
      <span className='text-muted-foreground hidden max-w-36 truncate text-sm sm:inline'>
        {user.display_name}
      </span>
      <Button
        aria-label={t('Sign out')}
        disabled={isLoggingOut}
        onClick={onLogout}
        size='icon'
        title={t('Sign out')}
        variant='ghost'
      >
        <HugeiconsIcon icon={Logout01Icon} strokeWidth={2} />
      </Button>
    </header>
  )
}
