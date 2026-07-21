import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { FormField } from '@/components/ui/form-field'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import type { CollectionRunItem } from '@/features/sites/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { applyApiFieldErrors } from '@/lib/form-errors'

import {
  archiveAccount,
  deleteAccount,
  getAccount,
  refreshAccount,
  restoreAccount,
  updateAccount,
} from '../api'
import { accountKeys } from '../query-keys'
import { accountRemarkSchema, type AccountRemarkValues } from '../schema'
import type { AccountListItem } from '../types'
import type { AccountAction } from './account-ui'

export type AccountDialogState = {
  action: AccountAction
  account: AccountListItem
} | null

function EditAccountDialog({
  account,
  onClose,
  onSaved,
}: {
  account: AccountListItem
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  const detailQuery = useQuery({
    queryFn: () => getAccount(account.id),
    queryKey: accountKeys.detail(account.id),
    refetchOnMount: 'always',
    staleTime: 0,
  })
  const {
    formState: { errors },
    handleSubmit,
    register,
    reset,
    setError,
  } = useForm<AccountRemarkValues>({
    defaultValues: { remark: '' },
    resolver: zodResolver(accountRemarkSchema),
  })
  useEffect(() => {
    if (detailQuery.data) reset({ remark: detailQuery.data.remark })
  }, [detailQuery.data, reset])
  const submit = handleSubmit(async (values) => {
    setPending(true)
    try {
      await updateAccount(account.id, { remark: values.remark })
      toast.success(t('account.toast.updated'))
      onSaved()
      onClose()
    } catch (error) {
      const mapped = applyApiFieldErrors(error, setError, { remark: 'remark' })
      if (!mapped) {
        setError('root', { message: getApiErrorTranslationKey(error) })
      }
    } finally {
      setPending(false)
    }
  })
  let content: ReactNode
  if (detailQuery.isPending) {
    content = (
      <div className='flex min-h-32 items-center justify-center'>
        <Spinner />
      </div>
    )
  } else if (detailQuery.isError) {
    content = (
      <section className='grid gap-3'>
        <p className='text-destructive text-sm' role='alert'>
          {t('account.detail.loadError')}
        </p>
        <Button onClick={() => void detailQuery.refetch()} variant='outline'>
          {t('common.retry')}
        </Button>
      </section>
    )
  } else {
    content = (
      <form
        className='grid gap-4'
        id='account-edit-form'
        noValidate
        onSubmit={submit}
      >
        <FormField
          description={t('account.edit.bindingImmutable')}
          error={
            errors.remark?.type === 'server'
              ? errors.remark.message
              : errors.remark?.message &&
                t(dynamicI18nKey('account', errors.remark.message))
          }
          htmlFor='account-remark'
          label={t('account.remark')}
        >
          <Textarea
            className='min-h-28'
            id='account-remark'
            {...register('remark')}
          />
        </FormField>
        {errors.root?.message && (
          <p className='text-destructive text-sm' role='alert'>
            {t(dynamicI18nKey('account', errors.root.message))}
          </p>
        )}
      </form>
    )
  }
  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('account.edit.title')}</DialogTitle>
          <DialogDescription>{t('account.edit.description')}</DialogDescription>
        </DialogHeader>
        {content}
        <DialogFooter>
          <Button onClick={onClose} variant='outline'>
            {t('common.cancel')}
          </Button>
          <Button
            disabled={pending || !detailQuery.data}
            form='account-edit-form'
            type='submit'
          >
            {pending && <Spinner />}
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function AccountDialogs({
  onClose,
  onRecovery,
  onSaved,
  state,
}: {
  onClose: () => void
  onRecovery: (run: CollectionRunItem, account: AccountListItem) => void
  onSaved: () => void
  state: AccountDialogState
}) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  if (!state) return null
  if (state.action === 'edit') {
    return (
      <EditAccountDialog
        account={state.account}
        onClose={onClose}
        onSaved={onSaved}
      />
    )
  }
  const execute = async () => {
    setPending(true)
    try {
      if (state.action === 'archive') await archiveAccount(state.account.id)
      else if (state.action === 'delete') await deleteAccount(state.account.id)
      else if (state.action === 'refresh') {
        await refreshAccount(state.account.id)
      } else {
        const run = await restoreAccount(state.account.id)
        onRecovery(run, state.account)
      }
      toast.success(
        t(dynamicI18nKey('account', `account.toast.${state.action}`))
      )
      onSaved()
      onClose()
    } catch (error) {
      toast.error(
        t(dynamicI18nKey('account', getApiErrorTranslationKey(error)))
      )
    } finally {
      setPending(false)
    }
  }
  return (
    <ConfirmDialog
      confirmLabel={t(
        dynamicI18nKey('account', `account.confirm.${state.action}.action`)
      )}
      description={t(
        dynamicI18nKey(
          'account',
          `account.confirm.${state.action}.description`
        ),
        {
          name: state.account.username,
        }
      )}
      onConfirm={() => void execute()}
      onOpenChange={(open) => !open && onClose()}
      open
      pending={pending}
      title={t(
        dynamicI18nKey('account', `account.confirm.${state.action}.title`)
      )}
      variant={
        state.action === 'restore' || state.action === 'refresh'
          ? 'primary'
          : 'destructive'
      }
    />
  )
}
