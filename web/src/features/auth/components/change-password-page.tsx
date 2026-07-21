import { zodResolver } from '@hookform/resolvers/zod'
import { useRouter } from '@tanstack/react-router'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { FormField } from '@/components/ui/form-field'
import { PasswordInput } from '@/components/ui/password-input'
import { Spinner } from '@/components/ui/spinner'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey, normalizeApiError } from '@/lib/api'
import { applyApiFieldErrors } from '@/lib/form-errors'
import { useAuthStore } from '@/stores/auth-store'

import { changePassword } from '../api'
import { changePasswordSchema, type ChangePasswordFormValues } from '../schema'
import { markSessionVerified } from '../session'

export function ChangePasswordPage() {
  const { t } = useTranslation()
  const router = useRouter()
  const user = useAuthStore((state) => state.user)
  const [submitting, setSubmitting] = useState(false)
  const {
    formState: { errors },
    handleSubmit,
    register,
    setError,
  } = useForm<ChangePasswordFormValues>({
    defaultValues: {
      confirmPassword: '',
      newPassword: '',
      originalPassword: '',
    },
    resolver: zodResolver(changePasswordSchema),
  })

  const submit = handleSubmit(async (values) => {
    setSubmitting(true)
    try {
      await changePassword({
        new_password: values.newPassword,
        original_password: values.originalPassword,
      })
      if (user) markSessionVerified({ ...user, must_change_password: false })
      toast.success(t('Password changed'))
      void router.navigate({ to: '/dashboard' })
    } catch (error) {
      const apiError = normalizeApiError(error)
      if (apiError.fieldErrors?.original_password) {
        setError('originalPassword', {
          message: 'Current password is incorrect',
          type: 'server',
        })
      } else if (
        !applyApiFieldErrors(error, setError, {
          new_password: 'newPassword',
          original_password: 'originalPassword',
        })
      ) {
        setError('root', {
          message: t(dynamicI18nKey('auth', getApiErrorTranslationKey(error))),
          type: 'server',
        })
      }
    } finally {
      setSubmitting(false)
    }
  })

  return (
    <main
      className='flex min-h-full items-center justify-center px-4 py-8'
      id='main-content'
    >
      <section className='bg-card border-border w-full max-w-lg rounded-2xl border p-5 shadow-lg shadow-black/5 sm:p-6'>
        <h1 className='text-lg font-semibold'>{t('Change password')}</h1>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t('A new password is required before you can use the workspace')}
        </p>
        <form className='mt-5 grid gap-4' noValidate onSubmit={submit}>
          <FormField
            error={
              errors.originalPassword?.message ===
              'Current password is incorrect'
                ? t('Current password is incorrect')
                : errors.originalPassword?.message &&
                  t(dynamicI18nKey('auth', errors.originalPassword.message))
            }
            htmlFor='original-password'
            label={t('Current password')}
            required
          >
            <PasswordInput
              aria-invalid={Boolean(errors.originalPassword)}
              autoComplete='current-password'
              id='original-password'
              {...register('originalPassword')}
            />
          </FormField>
          <FormField
            description={t(
              'Use 8 or more Unicode characters, up to 72 UTF-8 bytes'
            )}
            error={
              errors.newPassword?.message &&
              t(dynamicI18nKey('auth', errors.newPassword.message))
            }
            htmlFor='new-password'
            label={t('New password')}
            required
          >
            <PasswordInput
              aria-invalid={Boolean(errors.newPassword)}
              autoComplete='new-password'
              id='new-password'
              {...register('newPassword')}
            />
          </FormField>
          <FormField
            error={
              errors.confirmPassword?.message &&
              t(dynamicI18nKey('auth', errors.confirmPassword.message))
            }
            htmlFor='confirm-password'
            label={t('Confirm password')}
            required
          >
            <PasswordInput
              aria-invalid={Boolean(errors.confirmPassword)}
              autoComplete='new-password'
              id='confirm-password'
              {...register('confirmPassword')}
            />
          </FormField>
          {errors.root?.message && (
            <p className='text-destructive text-sm' role='alert'>
              {errors.root.message}
            </p>
          )}
          <div className='flex justify-end'>
            <Button disabled={submitting} type='submit'>
              {submitting && <Spinner />}
              {t('Change password')}
            </Button>
          </div>
        </form>
      </section>
    </main>
  )
}
