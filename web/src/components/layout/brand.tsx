import { ChartRelationshipIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

export function Brand({ variant = 'auth' }: { variant?: 'auth' | 'inline' }) {
  const { t } = useTranslation()

  if (variant === 'inline') {
    return (
      <Link
        aria-label={t('Go to home')}
        className={cn(
          'text-foreground inline-flex h-7 items-center gap-1.5 rounded-md px-1.5 text-sm font-medium transition-colors outline-none select-none',
          'hover:bg-accent focus-visible:ring-ring/40 focus-visible:ring-2'
        )}
        to='/dashboard'
      >
        <span className='bg-primary text-primary-foreground flex size-5 items-center justify-center overflow-hidden rounded-md'>
          <HugeiconsIcon
            icon={ChartRelationshipIcon}
            size={14}
            strokeWidth={2}
          />
        </span>
        <span className='max-w-[12rem] truncate'>{t('app.name')}</span>
      </Link>
    )
  }

  return (
    <Link
      aria-label={t('Go to home')}
      className='flex min-h-10 items-center gap-2 transition-opacity hover:opacity-80'
      to='/sign-in'
    >
      <span className='bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-full'>
        <HugeiconsIcon icon={ChartRelationshipIcon} size={20} strokeWidth={2} />
      </span>
      <span className='text-xl font-medium'>{t('app.name')}</span>
    </Link>
  )
}
