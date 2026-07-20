import { useTranslation } from 'react-i18next'

import type { LoginUser } from '@/features/auth/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import { Badge } from '../ui/badge'
import { AppNav } from './app-nav'
import { Brand } from './brand'

export function AppSidebar({
  onNavigate,
  user,
}: {
  onNavigate?: () => void
  user: LoginUser
}) {
  const { t } = useTranslation()

  return (
    <div className='flex h-full flex-col'>
      <div className='flex h-14 shrink-0 items-center border-b px-4'>
        <Brand />
      </div>
      <div className='flex-1 overflow-y-auto p-3'>
        <AppNav onNavigate={onNavigate} />
      </div>
      <div className='border-t p-3'>
        <div className='min-w-0 rounded-md px-2 py-2'>
          <div className='truncate text-sm font-medium'>
            {user.display_name}
          </div>
          <div className='mt-1 flex items-center justify-between gap-2'>
            <span className='text-muted-foreground truncate text-xs'>
              {user.username}
            </span>
            <Badge variant={user.role === 'admin' ? 'primary' : 'neutral'}>
              {t(
                dynamicI18nKey(
                  'layout',
                  user.role === 'admin' ? 'Administrator' : 'Viewer'
                )
              )}
            </Badge>
          </div>
        </div>
      </div>
    </div>
  )
}
