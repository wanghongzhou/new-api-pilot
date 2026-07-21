import { useRouter, type ErrorComponentProps } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey, normalizeApiError } from '@/lib/api'

export function AuthPendingState() {
  const { t } = useTranslation()
  return (
    <main className='flex min-h-svh items-center justify-center p-6'>
      <div aria-live='polite' className='flex items-center gap-2 text-sm'>
        <Spinner />
        <span>{t('Checking session')}</span>
      </div>
    </main>
  )
}

export function AuthErrorState({ error }: ErrorComponentProps) {
  const { t } = useTranslation()
  const router = useRouter()
  const apiError = normalizeApiError(error)

  return (
    <main className='flex min-h-svh items-center justify-center p-6'>
      <section className='border-border bg-card w-full max-w-md rounded-lg border p-5'>
        <h1 className='text-lg font-semibold'>
          {t('Unable to verify session')}
        </h1>
        <p className='text-muted-foreground mt-2 text-sm'>
          {t(dynamicI18nKey('auth', getApiErrorTranslationKey(apiError)))}
        </p>
        {apiError.requestId && (
          <p className='text-muted-foreground mt-2 text-xs'>
            {t('Request ID')}: {apiError.requestId}
          </p>
        )}
        <Button className='mt-4' onClick={() => void router.invalidate()}>
          {t('Retry')}
        </Button>
      </section>
    </main>
  )
}
