import { zodResolver } from '@hookform/resolvers/zod'
import { useRouter } from '@tanstack/react-router'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Brand } from '@/components/layout/brand'
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
    <main className='relative grid h-svh max-w-none'>
      <div className='absolute top-4 left-4 z-10 transition-opacity hover:opacity-80 sm:top-8 sm:left-8'>
        <Brand />
      </div>
      <div className='container flex items-center pt-16 sm:pt-0'>
        <section className='mx-auto flex w-full flex-col justify-center space-y-8 px-4 py-8 sm:w-[480px] sm:p-8'>
          <div className='space-y-2'>
            <h1 className='text-center text-2xl font-semibold tracking-tight sm:text-left'>
              {t('Sign in')}
            </h1>
            <p className='text-muted-foreground text-left text-sm sm:text-base'>
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
            <Button
              className='bg-primary-strong hover:bg-primary-strong-hover mt-2 w-full text-white'
              disabled={submitting}
              type='submit'
            >
              {submitting && <Spinner />}
              {t('Sign in')}
            </Button>
          </form>
        </section>
      </div>
    </main>
  )
}
