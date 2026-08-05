import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

export function QueryStateAlert({
  message,
  onRetry,
  tone = 'warning',
}: {
  message: string
  onRetry: () => void
  tone?: 'destructive' | 'warning'
}) {
  const { t } = useTranslation()
  return (
    <div
      className={
        tone === 'destructive'
          ? 'border-destructive/35 bg-destructive/5 text-destructive flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3 py-2 text-sm'
          : 'border-warning/40 bg-warning/10 text-warning-foreground flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3 py-2 text-sm'
      }
      role='alert'
    >
      <span>{message}</span>
      <Button onClick={onRetry} size='sm' type='button' variant='outline'>
        {t('common.retry')}
      </Button>
    </div>
  )
}
