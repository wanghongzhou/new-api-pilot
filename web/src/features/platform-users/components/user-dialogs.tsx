import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useRef, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Dialog,
  DialogCancelButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from '@/components/ui/drawer'
import { FormField } from '@/components/ui/form-field'
import { Input } from '@/components/ui/input'
import { PasswordInput } from '@/components/ui/password-input'
import { SelectControl as Select } from '@/components/ui/select-control'
import { Spinner } from '@/components/ui/spinner'
import { UnsavedChangesConfirmDialog } from '@/components/unsaved-changes-guard'
import { useUnsavedChangesGuard } from '@/hooks/use-unsaved-changes-guard'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey, normalizeApiError } from '@/lib/api'
import { applyApiFieldErrors } from '@/lib/form-errors'
import { useAuthStore } from '@/stores/auth-store'

import {
  createPlatformUser,
  disablePlatformUser,
  enablePlatformUser,
  resetPlatformUserPassword,
  updatePlatformUser,
} from '../api'
import {
  createPlatformUserSchema,
  editPlatformUserSchema,
  resetPlatformUserPasswordSchema,
  type CreatePlatformUserFormValues,
  type EditPlatformUserFormValues,
  type ResetPlatformUserPasswordFormValues,
} from '../schema'
import type { PlatformUserItem } from '../types'

interface ControlledDialogProps {
  disabledReason?: string
  onOpenChange: (open: boolean) => void
  onSaved: () => void
  open: boolean
}

function translatedError(
  message: string | undefined,
  t: (key: string) => string
) {
  return message ? t(dynamicI18nKey('platformUser', message)) : undefined
}

function DisabledWriteNotice({ reason }: { reason?: string }) {
  if (!reason) return null
  return (
    <p
      className='border-warning/40 bg-warning/10 text-warning-foreground rounded-md border px-3 py-2 text-sm'
      role='alert'
    >
      {reason}
    </p>
  )
}

export function CreateUserDialog({
  disabledReason,
  onOpenChange,
  onSaved,
  open,
}: ControlledDialogProps) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)
  const submittingRef = useRef(false)
  const {
    control,
    formState: { errors, isDirty },
    handleSubmit,
    register,
    reset,
    setError,
  } = useForm<CreatePlatformUserFormValues>({
    defaultValues: {
      confirmPassword: '',
      displayName: '',
      password: '',
      role: 'viewer',
      username: '',
    },
    resolver: zodResolver(createPlatformUserSchema),
  })
  const createCloseGuard = useUnsavedChangesGuard({
    hasUnsavedChanges: isDirty,
    onClose: () => onOpenChange(false),
  })

  useEffect(() => {
    if (open) {
      reset({
        confirmPassword: '',
        displayName: '',
        password: '',
        role: 'viewer',
        username: '',
      })
    }
  }, [open, reset])

  const submit = handleSubmit(async (values) => {
    if (disabledReason || submittingRef.current) return
    submittingRef.current = true
    setSubmitting(true)
    try {
      await createPlatformUser({
        display_name: values.displayName,
        password: values.password,
        role: values.role,
        username: values.username,
      })
      toast.success(t('Platform user created'))
      onSaved()
      onOpenChange(false)
    } catch (error) {
      const mapped = applyApiFieldErrors(error, setError, {
        display_name: 'displayName',
      })
      if (!mapped) {
        setError('root', {
          message: t(
            dynamicI18nKey('platformUser', getApiErrorTranslationKey(error))
          ),
          type: 'server',
        })
      }
    } finally {
      submittingRef.current = false
      setSubmitting(false)
    }
  })

  return (
    <>
      <Drawer
        direction='right'
        onOpenChange={(nextOpen) => {
          if (nextOpen) onOpenChange(true)
          else if (submitting) return
          else createCloseGuard.requestClose()
        }}
        open={open && !createCloseGuard.confirmOpen}
      >
        <DrawerContent className={sideDrawerContentClassName('sm:max-w-xl')}>
          <DrawerHeader className={sideDrawerHeaderClassName()}>
            <DrawerTitle>{t('Create platform user')}</DrawerTitle>
            <DrawerDescription>
              {t('New users must change their password after signing in')}
            </DrawerDescription>
          </DrawerHeader>
          <form
            className={sideDrawerFormClassName('gap-4')}
            id='create-user-form'
            onSubmit={submit}
          >
            <DisabledWriteNotice reason={disabledReason} />
            <FormField
              error={translatedError(errors.username?.message, t)}
              htmlFor='create-username'
              label={t('Username')}
              required
            >
              <Input
                aria-invalid={Boolean(errors.username)}
                autoCapitalize='none'
                autoComplete='off'
                id='create-username'
                {...register('username')}
              />
            </FormField>
            <FormField
              error={translatedError(errors.displayName?.message, t)}
              htmlFor='create-display-name'
              label={t('Display name')}
              required
            >
              <Input
                aria-invalid={Boolean(errors.displayName)}
                id='create-display-name'
                {...register('displayName')}
              />
            </FormField>
            <FormField htmlFor='create-role' label={t('Role')} required>
              <Controller
                control={control}
                name='role'
                render={({ field }) => (
                  <Select
                    alignItemWithTrigger={false}
                    id='create-role'
                    name={field.name}
                    onChange={(event) => field.onChange(event.target.value)}
                    portalled={false}
                    value={field.value}
                  >
                    <option value='viewer'>{t('Viewer')}</option>
                    <option value='admin'>{t('Administrator')}</option>
                  </Select>
                )}
              />
            </FormField>
            <FormField
              description={t(
                'Use 8 or more Unicode characters, up to 72 UTF-8 bytes'
              )}
              error={translatedError(errors.password?.message, t)}
              htmlFor='create-password'
              label={t('Temporary password')}
              required
            >
              <PasswordInput
                aria-invalid={Boolean(errors.password)}
                autoComplete='new-password'
                id='create-password'
                {...register('password')}
              />
            </FormField>
            <FormField
              error={translatedError(errors.confirmPassword?.message, t)}
              htmlFor='create-confirm-password'
              label={t('Confirm password')}
              required
            >
              <PasswordInput
                aria-invalid={Boolean(errors.confirmPassword)}
                autoComplete='new-password'
                id='create-confirm-password'
                {...register('confirmPassword')}
              />
            </FormField>
            {errors.root?.message && (
              <p className='text-destructive text-sm' role='alert'>
                {errors.root.message}
              </p>
            )}
          </form>
          <DrawerFooter className={sideDrawerFooterClassName()}>
            <Button
              disabled={submitting}
              onClick={createCloseGuard.requestClose}
              type='button'
              variant='outline'
            >
              {t('common.cancel')}
            </Button>
            <Button
              disabled={submitting || Boolean(disabledReason)}
              form='create-user-form'
              type='submit'
            >
              {submitting && <Spinner />}
              {t('Create user')}
            </Button>
          </DrawerFooter>
        </DrawerContent>
      </Drawer>
      <UnsavedChangesConfirmDialog
        onConfirm={createCloseGuard.discardAndClose}
        onOpenChange={createCloseGuard.setConfirmOpen}
        open={createCloseGuard.confirmOpen}
      />
    </>
  )
}

export function EditUserDialog({
  disabledReason,
  isLastEnabledAdmin,
  onOpenChange,
  onSaved,
  open,
  user,
}: ControlledDialogProps & {
  isLastEnabledAdmin: boolean
  user: PlatformUserItem | null
}) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)
  const submittingRef = useRef(false)
  const {
    control,
    formState: { errors, isDirty },
    handleSubmit,
    register,
    reset,
    setError,
  } = useForm<EditPlatformUserFormValues>({
    defaultValues: { displayName: '', role: 'viewer', username: '' },
    resolver: zodResolver(editPlatformUserSchema),
  })
  const editCloseGuard = useUnsavedChangesGuard({
    hasUnsavedChanges: isDirty,
    onClose: () => onOpenChange(false),
  })

  useEffect(() => {
    if (open && user) {
      reset({
        displayName: user.display_name,
        role: user.role,
        username: user.username,
      })
    }
  }, [open, reset, user])

  const submit = handleSubmit(async (values) => {
    if (!user || disabledReason || submittingRef.current) return
    submittingRef.current = true
    setSubmitting(true)
    try {
      const updated = await updatePlatformUser(user.id, {
        display_name: values.displayName,
        expected_updated_at: user.updated_at,
        role: values.role,
        username: values.username,
      })
      if (useAuthStore.getState().user?.id === updated.id) {
        useAuthStore.getState().setUser({
          display_name: updated.display_name,
          id: updated.id,
          must_change_password: updated.must_change_password,
          role: updated.role,
          status: updated.status,
          username: updated.username,
        })
      }
      toast.success(t('Platform user updated'))
      onSaved()
      onOpenChange(false)
    } catch (error) {
      const mapped = applyApiFieldErrors(error, setError, {
        display_name: 'displayName',
      })
      if (!mapped) {
        const apiError = normalizeApiError(error)
        const isConcurrentEdit =
          apiError.status === 409 && apiError.code === 'CONFLICT'
        const message = isConcurrentEdit
          ? t('Platform user changed; refresh and retry')
          : t(dynamicI18nKey('platformUser', getApiErrorTranslationKey(error)))
        setError('root', {
          message,
          type: 'server',
        })
        if (isConcurrentEdit) toast.error(message)
      }
    } finally {
      submittingRef.current = false
      setSubmitting(false)
    }
  })

  return (
    <>
      <Drawer
        direction='right'
        onOpenChange={(nextOpen) => {
          if (nextOpen) onOpenChange(true)
          else if (submitting) return
          else editCloseGuard.requestClose()
        }}
        open={open && !editCloseGuard.confirmOpen}
      >
        <DrawerContent className={sideDrawerContentClassName('sm:max-w-xl')}>
          <DrawerHeader className={sideDrawerHeaderClassName()}>
            <DrawerTitle>{t('Edit platform user')}</DrawerTitle>
            <DrawerDescription>
              {t('Role changes invalidate the user session')}
            </DrawerDescription>
          </DrawerHeader>
          <form
            className={sideDrawerFormClassName('gap-4')}
            id='edit-user-form'
            onSubmit={submit}
          >
            <DisabledWriteNotice reason={disabledReason} />
            <FormField
              error={translatedError(errors.username?.message, t)}
              htmlFor='edit-username'
              label={t('Username')}
              required
            >
              <Input
                aria-invalid={Boolean(errors.username)}
                autoCapitalize='none'
                id='edit-username'
                {...register('username')}
              />
            </FormField>
            <FormField
              error={translatedError(errors.displayName?.message, t)}
              htmlFor='edit-display-name'
              label={t('Display name')}
              required
            >
              <Input
                aria-invalid={Boolean(errors.displayName)}
                id='edit-display-name'
                {...register('displayName')}
              />
            </FormField>
            <FormField
              description={
                isLastEnabledAdmin
                  ? t('The last enabled administrator cannot be downgraded')
                  : undefined
              }
              htmlFor='edit-role'
              label={t('Role')}
              required
            >
              <Controller
                control={control}
                name='role'
                render={({ field }) => (
                  <Select
                    alignItemWithTrigger={false}
                    id='edit-role'
                    name={field.name}
                    onChange={(event) => field.onChange(event.target.value)}
                    portalled={false}
                    value={field.value}
                  >
                    <option
                      disabled={isLastEnabledAdmin && user?.role === 'admin'}
                      value='viewer'
                    >
                      {t('Viewer')}
                    </option>
                    <option value='admin'>{t('Administrator')}</option>
                  </Select>
                )}
              />
            </FormField>
            {errors.root?.message && (
              <p className='text-destructive text-sm' role='alert'>
                {errors.root.message}
              </p>
            )}
          </form>
          <DrawerFooter className={sideDrawerFooterClassName()}>
            <Button
              disabled={submitting}
              onClick={editCloseGuard.requestClose}
              type='button'
              variant='outline'
            >
              {t('common.cancel')}
            </Button>
            <Button
              disabled={submitting || Boolean(disabledReason)}
              form='edit-user-form'
              type='submit'
            >
              {submitting && <Spinner />}
              {t('Save changes')}
            </Button>
          </DrawerFooter>
        </DrawerContent>
      </Drawer>
      <UnsavedChangesConfirmDialog
        onConfirm={editCloseGuard.discardAndClose}
        onOpenChange={editCloseGuard.setConfirmOpen}
        open={editCloseGuard.confirmOpen}
      />
    </>
  )
}

export function ResetPasswordDialog({
  disabledReason,
  onOpenChange,
  onSaved,
  open,
  user,
}: ControlledDialogProps & { user: PlatformUserItem | null }) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)
  const submittingRef = useRef(false)
  const {
    formState: { errors, isDirty },
    handleSubmit,
    register,
    reset,
    setError,
  } = useForm<ResetPlatformUserPasswordFormValues>({
    defaultValues: { confirmPassword: '', password: '' },
    resolver: zodResolver(resetPlatformUserPasswordSchema),
  })
  const resetCloseGuard = useUnsavedChangesGuard({
    hasUnsavedChanges: isDirty,
    onClose: () => onOpenChange(false),
  })

  useEffect(() => {
    if (open) reset({ confirmPassword: '', password: '' })
  }, [open, reset])

  const submit = handleSubmit(async (values) => {
    if (!user || disabledReason || submittingRef.current) return
    submittingRef.current = true
    setSubmitting(true)
    try {
      await resetPlatformUserPassword(user.id, {
        new_password: values.password,
      })
      toast.success(t('Password reset'))
      onSaved()
      onOpenChange(false)
    } catch (error) {
      const mapped = applyApiFieldErrors(error, setError, {
        new_password: 'password',
      })
      if (!mapped) {
        setError('root', {
          message: t(
            dynamicI18nKey('platformUser', getApiErrorTranslationKey(error))
          ),
          type: 'server',
        })
      }
    } finally {
      submittingRef.current = false
      setSubmitting(false)
    }
  })

  return (
    <>
      <Dialog
        onOpenChange={(nextOpen) => {
          if (nextOpen) onOpenChange(true)
          else if (submitting) return
          else resetCloseGuard.requestClose()
        }}
        open={open && !resetCloseGuard.confirmOpen}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Reset password')}</DialogTitle>
            <DialogDescription>
              {t('{{username}} must change this password at the next sign-in', {
                username: user?.username ?? '',
              })}
            </DialogDescription>
          </DialogHeader>
          <DisabledWriteNotice reason={disabledReason} />
          <form
            className='grid gap-4'
            id='reset-password-form'
            onSubmit={submit}
          >
            <FormField
              description={t(
                'Use 8 or more Unicode characters, up to 72 UTF-8 bytes'
              )}
              error={translatedError(errors.password?.message, t)}
              htmlFor='reset-password'
              label={t('Temporary password')}
              required
            >
              <PasswordInput
                aria-invalid={Boolean(errors.password)}
                autoComplete='new-password'
                id='reset-password'
                {...register('password')}
              />
            </FormField>
            <FormField
              error={translatedError(errors.confirmPassword?.message, t)}
              htmlFor='reset-confirm-password'
              label={t('Confirm password')}
              required
            >
              <PasswordInput
                aria-invalid={Boolean(errors.confirmPassword)}
                autoComplete='new-password'
                id='reset-confirm-password'
                {...register('confirmPassword')}
              />
            </FormField>
            {errors.root?.message && (
              <p className='text-destructive text-sm' role='alert'>
                {errors.root.message}
              </p>
            )}
          </form>
          <DialogFooter>
            <DialogCancelButton disabled={submitting} />
            <Button
              disabled={submitting || Boolean(disabledReason)}
              form='reset-password-form'
              type='submit'
            >
              {submitting && <Spinner />}
              {t('Reset password')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <UnsavedChangesConfirmDialog
        onConfirm={resetCloseGuard.discardAndClose}
        onOpenChange={resetCloseGuard.setConfirmOpen}
        open={resetCloseGuard.confirmOpen}
      />
    </>
  )
}

export function ToggleUserDialog({
  action,
  disabledReason,
  onOpenChange,
  onSaved,
  open,
  user,
}: ControlledDialogProps & {
  action: 'enable' | 'disable'
  user: PlatformUserItem | null
}) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)
  const submittingRef = useRef(false)

  const submit = async () => {
    if (!user || disabledReason || submittingRef.current) return
    submittingRef.current = true
    setSubmitting(true)
    try {
      if (action === 'enable') await enablePlatformUser(user.id)
      else await disablePlatformUser(user.id)
      toast.success(
        t(
          dynamicI18nKey(
            'platformUser',
            action === 'enable'
              ? 'Platform user enabled'
              : 'Platform user disabled'
          )
        )
      )
      onSaved()
      onOpenChange(false)
    } catch (error) {
      toast.error(
        t(dynamicI18nKey('platformUser', getApiErrorTranslationKey(error)))
      )
    } finally {
      submittingRef.current = false
      setSubmitting(false)
    }
  }

  const disabling = action === 'disable'
  if (disabling) {
    return (
      <ConfirmDialog
        confirmDisabled={Boolean(disabledReason)}
        confirmLabel={t('Disable user')}
        description={
          <div className='grid gap-3'>
            <span>
              {t('{{username}} will immediately lose access to the platform', {
                username: user?.username ?? '',
              })}
            </span>
            <DisabledWriteNotice reason={disabledReason} />
          </div>
        }
        onConfirm={() => void submit()}
        onOpenChange={onOpenChange}
        open={open}
        pending={submitting}
        title={t('Disable platform user')}
      />
    )
  }
  return (
    <Dialog
      onOpenChange={(nextOpen) => {
        if (nextOpen) onOpenChange(true)
        else if (!submitting) onOpenChange(false)
      }}
      open={open}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Enable platform user')}</DialogTitle>
          <DialogDescription>
            {t('{{username}} will be allowed to sign in again', {
              username: user?.username ?? '',
            })}
          </DialogDescription>
        </DialogHeader>
        <DisabledWriteNotice reason={disabledReason} />
        <DialogFooter>
          <DialogCancelButton disabled={submitting} />
          <Button
            disabled={submitting || Boolean(disabledReason)}
            onClick={() => void submit()}
            variant='default'
          >
            {submitting && <Spinner />}
            {t('Enable user')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
