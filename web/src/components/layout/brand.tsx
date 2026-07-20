import { ChartRelationshipIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

export function Brand({ compact = false }: { compact?: boolean }) {
  const { t } = useTranslation()
  return (
    <div className='flex min-w-0 items-center gap-2.5'>
      <span className='bg-primary text-primary-foreground flex size-9 shrink-0 items-center justify-center rounded-md'>
        <HugeiconsIcon icon={ChartRelationshipIcon} strokeWidth={2} />
      </span>
      <span
        className={cn(
          'truncate text-sm font-semibold',
          compact && 'hidden sm:inline'
        )}
      >
        {t('app.name')}
      </span>
    </div>
  )
}
