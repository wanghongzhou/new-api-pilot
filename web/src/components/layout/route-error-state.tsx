import { Alert02Icon, FileNotFoundIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  Link,
  useRouter,
  type ErrorComponentProps,
} from '@tanstack/react-router'
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
import { normalizeApiError } from '@/lib/api'

export function RouteErrorState({ error, reset }: ErrorComponentProps) {
  const { t } = useTranslation()
  const router = useRouter()
  const apiError = normalizeApiError(error)

  const retry = () => {
    reset()
    void router.invalidate()
  }

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
          <EmptyTitle>{t('Page failed to load')}</EmptyTitle>
          <EmptyDescription>
            {t('An unexpected error occurred. Please try again.')}
          </EmptyDescription>
          {apiError.requestId && (
            <p className='text-muted-foreground text-xs'>
              {t('Request ID')}: {apiError.requestId}
            </p>
          )}
        </EmptyHeader>
        <EmptyContent className='flex-row justify-center'>
          <Button render={<Link to='/dashboard' />} size='sm'>
            {t('Back to dashboard')}
          </Button>
          <Button onClick={retry} size='sm' variant='outline'>
            {t('Retry')}
          </Button>
        </EmptyContent>
      </Empty>
    </main>
  )
}

export function RouteNotFoundState() {
  const { t } = useTranslation()
  const router = useRouter()

  return (
    <main className='flex min-h-svh items-center justify-center p-6'>
      <Empty className='border-border bg-background min-h-[300px] max-w-md border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon
              className='text-muted-foreground size-6'
              icon={FileNotFoundIcon}
              strokeWidth={2}
            />
          </EmptyMedia>
          <h1
            className='text-sm font-medium tracking-tight'
            data-slot='empty-title'
          >
            {t('Page not found')}
          </h1>
          <EmptyDescription>
            {t('The page you requested does not exist or has been moved.')}
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent className='flex-row flex-wrap justify-center'>
          <Button
            onClick={() => router.history.back()}
            size='sm'
            variant='outline'
          >
            {t('Go back')}
          </Button>
          <Button render={<Link to='/dashboard' />} size='sm'>
            {t('Back to dashboard')}
          </Button>
        </EmptyContent>
      </Empty>
    </main>
  )
}
