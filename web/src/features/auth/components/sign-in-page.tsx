import { zodResolver } from '@hookform/resolvers/zod'
import { LockPasswordIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useRouter } from '@tanstack/react-router'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Brand } from '@/components/layout/brand'
import { ThemeToggle } from '@/components/layout/theme-toggle'
import { Button } from '@/components/ui/button'
import { FormField } from '@/components/ui/form-field'
import { Input } from '@/components/ui/input'
import { PasswordInput } from '@/components/ui/password-input'
import { Spinner } from '@/components/ui/spinner'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey, normalizeApiError } from '@/lib/api'

import { login } from '../api'
import { loginSchema, type LoginFormValues } from '../schema'
import { markSessionVerified } from '../session'

function safeRedirect(redirect: string | undefined): string {
  if (!redirect || !redirect.startsWith('/') || redirect.startsWith('//')) {
    return '/dashboard'
  }
  return redirect
}

export function SignInPage({ redirect }: { redirect?: string }) {
  const { t } = useTranslation()
  const router = useRouter()
  const [submitting, setSubmitting] = useState(false)
  const {
    formState: { errors },
    handleSubmit,
    register,
    setError,
  } = useForm<LoginFormValues>({
    defaultValues: { password: '', username: '' },
    resolver: zodResolver(loginSchema),
  })

  const submit = handleSubmit(async (values) => {
    setSubmitting(true)
    try {
      const user = await login(values)
      markSessionVerified(user)
      const destination = user.must_change_password
        ? '/change-password'
        : safeRedirect(redirect)
      router.history.push(destination)
    } catch (error) {
      const apiError = normalizeApiError(error)
      let message: string
      if (apiError.status === 401 || apiError.code === 'AUTH_INVALID') {
        message = t('Incorrect username or password')
      } else if (apiError.code === 'USER_DISABLED') {
        message = t('USER_DISABLED')
      } else {
        message = t(dynamicI18nKey('auth', getApiErrorTranslationKey(apiError)))
      }
      setError('root', {
        message,
        type: 'server',
      })
    } finally {
      setSubmitting(false)
    }
  })

  return (
    <main className='bg-muted/35 relative flex min-h-svh items-center justify-center px-4 py-10'>
      <div className='absolute top-3 right-3'>
        <ThemeToggle />
      </div>
      <section className='bg-card border-border w-full max-w-sm rounded-lg border p-6 shadow-sm'>
        <div className='mb-6'>
          <Brand />
          <div className='mt-5 flex items-center gap-2'>
            <HugeiconsIcon
              aria-hidden='true'
              className='text-primary size-5'
              icon={LockPasswordIcon}
              strokeWidth={2}
            />
            <h1 className='text-lg font-semibold'>{t('Sign in')}</h1>
          </div>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('Use your platform account to continue')}
          </p>
        </div>
        <form className='grid gap-4' noValidate onSubmit={submit}>
          <FormField
            error={
              errors.username?.message &&
              t(dynamicI18nKey('auth', errors.username.message))
            }
            htmlFor='username'
            label={t('Username')}
            required
          >
            <Input
              aria-describedby={
                errors.username?.message ? 'username-error' : undefined
              }
              aria-invalid={Boolean(errors.username)}
              autoCapitalize='none'
              autoComplete='username'
              autoFocus
              id='username'
              {...register('username')}
            />
          </FormField>
          <FormField
            error={
              errors.password?.message &&
              t(dynamicI18nKey('auth', errors.password.message))
            }
            htmlFor='password'
            label={t('Password')}
            required
          >
            <PasswordInput
              aria-describedby={
                errors.password?.message ? 'password-error' : undefined
              }
              aria-invalid={Boolean(errors.password)}
              autoComplete='current-password'
              id='password'
              {...register('password')}
            />
          </FormField>
          {errors.root?.message && (
            <p className='text-destructive text-sm' role='alert'>
              {errors.root.message}
            </p>
          )}
          <Button className='mt-1 w-full' disabled={submitting} type='submit'>
            {submitting && <Spinner />}
            {t('Sign in')}
          </Button>
        </form>
      </section>
    </main>
  )
}
