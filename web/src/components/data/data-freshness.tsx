import { Clock01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { fromUnixSeconds } from '@/lib/dayjs'

export function DataFreshness({
  expired = false,
  labelKey,
  timestamp,
}: {
  expired?: boolean
  labelKey: string
  timestamp: number | null
}) {
  const { t } = useTranslation()
  if (timestamp == null) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('data.noUpdateTime')}
      </span>
    )
  }
  const exact = fromUnixSeconds(timestamp).format('YYYY-MM-DD HH:mm:ss')
  return (
    <span
      className='text-muted-foreground inline-flex items-center gap-1 text-xs'
      title={exact}
    >
      <HugeiconsIcon icon={Clock01Icon} size={14} strokeWidth={2} />
      {t(dynamicI18nKey('data', labelKey), { time: exact })}
      {expired && (
        <strong className='text-destructive font-medium'>
          {t('data.stale')}
        </strong>
      )}
    </span>
  )
}
