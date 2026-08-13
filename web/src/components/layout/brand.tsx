import { ChartRelationshipIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

export function Brand({ variant = 'auth' }: { variant?: 'auth' | 'inline' }) {
  const { t } = useTranslation()
  const router = useRouter()

  if (variant === 'inline') {
    return (
      <a
        aria-label={t('Go to home')}
        className={cn(
          'text-foreground inline-flex min-h-10 items-center gap-1.5 rounded-md px-1.5 text-sm font-medium transition-colors outline-none select-none sm:min-h-7',
          'hover:bg-accent focus-visible:ring-ring/40 focus-visible:ring-2'
        )}
        href='/dashboard'
        onClick={(event) => {
          event.preventDefault()
          void router.navigate({
            href: new URL('/dashboard', window.location.origin).href,
            reloadDocument: true,
          })
        }}
      >
        <span className='bg-primary text-primary-foreground flex size-5 items-center justify-center overflow-hidden rounded-md'>
          <HugeiconsIcon
            icon={ChartRelationshipIcon}
            size={14}
            strokeWidth={2}
          />
        </span>
        <span className='max-w-[12rem] truncate'>{t('app.name')}</span>
      </a>
    )
  }

  return (
    <a
      aria-label={t('Go to home')}
      className='flex min-h-10 items-center gap-2 transition-opacity hover:opacity-80'
      href='/sign-in'
      onClick={(event) => {
        event.preventDefault()
        void router.navigate({
          href: new URL('/sign-in', window.location.origin).href,
          reloadDocument: true,
        })
      }}
    >
      <span className='bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-full'>
        <HugeiconsIcon icon={ChartRelationshipIcon} size={20} strokeWidth={2} />
      </span>
      <span className='text-xl font-medium'>{t('app.name')}</span>
    </a>
  )
}
