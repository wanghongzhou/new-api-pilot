import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { FormField } from '@/components/ui/form-field'
import { Input } from '@/components/ui/input'
import { PasswordInput } from '@/components/ui/password-input'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Spinner } from '@/components/ui/spinner'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'
import { applyApiFieldErrors } from '@/lib/form-errors'

import { authorizeSite } from '../api'
import {
  siteAuthorizationFormSchema,
  type SiteAuthorizationValues,
} from '../schema'
import type { SiteAuthorizationResult } from '../types'

export function SiteAuthorizationForm({
  formId,
  onSuccess,
  siteId,
  submitLabel,
}: {
  formId: string
  onSuccess: (result: SiteAuthorizationResult) => void
  siteId: string
  submitLabel: string
}) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)
  const {
    control,
    formState: { errors },
    handleSubmit,
    register,
    reset,
    setError,
    setValue,
    watch,
  } = useForm<SiteAuthorizationValues>({
    defaultValues: {
      accessToken: '',
      confirmTokenRotation: false,
      mode: 'existing_token',
      password: '',
      rootUserId: '',
      username: '',
    },
    resolver: zodResolver(siteAuthorizationFormSchema),
    shouldUnregister: true,
  })
  const mode = watch('mode')

  useEffect(() => {
    if (mode === 'existing_token') {
      setValue('mode', 'existing_token')
    } else {
      setValue('mode', 'login_generate_token')
    }
  }, [mode, setValue])

  const submit = handleSubmit(async (values) => {
    setSubmitting(true)
    try {
      const result =
        values.mode === 'existing_token'
          ? await authorizeSite(parseIdString(siteId), {
              access_token: values.accessToken ?? '',
              mode: 'existing_token',
              root_user_id: parseIdString(values.rootUserId ?? ''),
            })
          : await authorizeSite(parseIdString(siteId), {
              confirm_token_rotation: true,
              mode: 'login_generate_token',
              password: values.password ?? '',
              username: values.username ?? '',
            })
      reset({
        accessToken: '',
        confirmTokenRotation: false,
        mode: 'existing_token',
        password: '',
        rootUserId: '',
        username: '',
      })
      onSuccess(result)
    } catch (error) {
      const mapped = applyApiFieldErrors(error, setError, {
        access_token: 'accessToken',
        confirm_token_rotation: 'confirmTokenRotation',
        root_user_id: 'rootUserId',
      })
      if (!mapped) {
        setError('root', {
          message: getApiErrorTranslationKey(error),
          type: 'server',
        })
      }
    } finally {
      setSubmitting(false)
    }
  })

  return (
    <form className='grid gap-4' id={formId} noValidate onSubmit={submit}>
      <fieldset className='grid gap-2'>
        <legend className='text-sm font-medium'>
          {t('site.authorization.mode')}
        </legend>
        <Controller
          control={control}
          name='mode'
          render={({ field }) => (
            <RadioGroup
              className='grid-cols-2'
              onValueChange={field.onChange}
              value={field.value}
            >
              <label
                className={
                  mode === 'existing_token'
                    ? 'border-primary bg-primary/8 flex min-h-12 items-center gap-2 rounded-md border px-3 text-sm font-medium'
                    : 'border-border hover:bg-muted flex min-h-12 items-center gap-2 rounded-md border px-3 text-sm'
                }
              >
                <RadioGroupItem value='existing_token' />
                {t('site.authorization.existingToken')}
              </label>
              <label
                className={
                  mode === 'login_generate_token'
                    ? 'border-primary bg-primary/8 flex min-h-12 items-center gap-2 rounded-md border px-3 text-sm font-medium'
                    : 'border-border hover:bg-muted flex min-h-12 items-center gap-2 rounded-md border px-3 text-sm'
                }
              >
                <RadioGroupItem value='login_generate_token' />
                {t('site.authorization.rotateToken')}
              </label>
            </RadioGroup>
          )}
        />
      </fieldset>

      {mode === 'existing_token' ? (
        <>
          <FormField
            error={
              errors.rootUserId?.message &&
              t(dynamicI18nKey('site', errors.rootUserId.message))
            }
            htmlFor={`${formId}-root-id`}
            label={t('site.authorization.rootUserId')}
            required
          >
            <Input
              aria-invalid={Boolean(errors.rootUserId)}
              autoComplete='off'
              id={`${formId}-root-id`}
              inputMode='numeric'
              {...register('rootUserId')}
            />
          </FormField>
          <FormField
            description={t('site.authorization.tokenNotRotated')}
            error={
              errors.accessToken?.message &&
              t(dynamicI18nKey('site', errors.accessToken.message))
            }
            htmlFor={`${formId}-token`}
            label={t('site.authorization.accessToken')}
            required
          >
            <PasswordInput
              aria-invalid={Boolean(errors.accessToken)}
              autoComplete='off'
              id={`${formId}-token`}
              {...register('accessToken')}
            />
          </FormField>
        </>
      ) : (
        <>
          <div className='border-warning/40 bg-warning/10 rounded-md border p-3 text-sm'>
            {t('site.authorization.rotationWarning')}
          </div>
          <FormField
            error={
              errors.username?.message &&
              t(dynamicI18nKey('site', errors.username.message))
            }
            htmlFor={`${formId}-username`}
            label={t('site.authorization.rootUsername')}
            required
          >
            <Input
              aria-invalid={Boolean(errors.username)}
              autoComplete='username'
              id={`${formId}-username`}
              {...register('username')}
            />
          </FormField>
          <FormField
            description={t('site.authorization.passwordNotStored')}
            error={
              errors.password?.message &&
              t(dynamicI18nKey('site', errors.password.message))
            }
            htmlFor={`${formId}-password`}
            label={t('site.authorization.rootPassword')}
            required
          >
            <PasswordInput
              aria-invalid={Boolean(errors.password)}
              autoComplete='current-password'
              id={`${formId}-password`}
              {...register('password')}
            />
          </FormField>
          <label className='border-border flex min-h-12 items-start gap-3 rounded-md border p-3 text-sm'>
            <Controller
              control={control}
              name='confirmTokenRotation'
              render={({ field }) => (
                <Checkbox
                  checked={field.value}
                  className='mt-0.5'
                  onBlur={field.onBlur}
                  onCheckedChange={field.onChange}
                  ref={field.ref}
                />
              )}
            />
            <span>{t('site.authorization.confirmRotation')}</span>
          </label>
          {errors.confirmTokenRotation?.message && (
            <p className='text-destructive text-xs' role='alert'>
              {t(dynamicI18nKey('site', errors.confirmTokenRotation.message))}
            </p>
          )}
        </>
      )}

      {errors.root?.message && (
        <p className='text-destructive text-sm' role='alert'>
          {t(dynamicI18nKey('site', errors.root.message))}
        </p>
      )}
      <Button disabled={submitting} type='submit'>
        {submitting && <Spinner />}
        {submitLabel}
      </Button>
    </form>
  )
}
