import { Alert02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useRouter, type ErrorComponentProps } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Spinner } from '@/components/ui/spinner'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey, normalizeApiError } from '@/lib/api'

export function AuthPendingState() {
  const { t } = useTranslation()
  return (
    <main className='flex min-h-svh items-center justify-center p-6'>
      <div
        aria-live='polite'
        className='flex min-h-52 flex-col items-center justify-center gap-3'
      >
        <Spinner className='size-6' />
        <span className='text-muted-foreground text-sm'>
          {t('Checking session')}
        </span>
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
      <Empty className='border-border bg-background min-h-[300px] max-w-md border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon
              className='text-destructive size-6'
              icon={Alert02Icon}
              strokeWidth={2}
            />
          </EmptyMedia>
          <EmptyTitle>{t('Unable to verify session')}</EmptyTitle>
          <EmptyDescription>
            {t(dynamicI18nKey('auth', getApiErrorTranslationKey(apiError)))}
          </EmptyDescription>
          {apiError.requestId && (
            <p className='text-muted-foreground text-xs'>
              {t('Request ID')}: {apiError.requestId}
            </p>
          )}
        </EmptyHeader>
        <EmptyContent>
          <Button
            onClick={() => void router.invalidate()}
            size='sm'
            variant='outline'
          >
            {t('Retry')}
          </Button>
        </EmptyContent>
      </Empty>
    </main>
  )
}
