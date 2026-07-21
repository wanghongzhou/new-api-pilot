import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
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
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import type { CollectionRunItem } from '@/features/sites/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { applyApiFieldErrors } from '@/lib/form-errors'

import {
  createCustomer,
  deleteCustomer,
  disableCustomer,
  enableCustomer,
  updateCustomer,
} from '../api'
import { editableCustomerStatuses } from '../constants'
import {
  customerFormSchema,
  type CustomerFormOutput,
  type CustomerFormValues,
} from '../schema'
import type { CustomerListItem } from '../types'
import type { CustomerAction } from './customer-ui'

export type CustomerDialogState =
  | { action: 'create'; customer?: undefined }
  | { action: CustomerAction; customer: CustomerListItem }
  | null

function CustomerFormDialog({
  customer,
  onClose,
  onSaved,
}: {
  customer?: CustomerListItem
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  const {
    formState: { errors },
    handleSubmit,
    register,
    setError,
  } = useForm<CustomerFormValues, unknown, CustomerFormOutput>({
    defaultValues: {
      contact: customer?.contact ?? '',
      contract_amount: customer?.contract_amount ?? '0',
      name: customer?.name ?? '',
      payment_amount: customer?.payment_amount ?? '0',
      remark: customer?.remark ?? '',
      status:
        customer?.status === 'communicating' ||
        customer?.status === 'signing' ||
        customer?.status === 'using'
          ? customer.status
          : 'communicating',
    },
    resolver: zodResolver(customerFormSchema),
  })
  const submit = handleSubmit(async (values) => {
    setPending(true)
    try {
      const request = {
        contact: values.contact || undefined,
        contract_amount: values.contract_amount || '0',
        name: values.name,
        payment_amount: values.payment_amount || '0',
        remark: values.remark || undefined,
        status: values.status,
      }
      if (customer) await updateCustomer(customer.id, request)
      else await createCustomer(request)
      toast.success(
        t(
          dynamicI18nKey(
            'customer',
            customer ? 'customer.toast.updated' : 'customer.toast.created'
          )
        )
      )
      onSaved()
      onClose()
    } catch (error) {
      const mapped = applyApiFieldErrors(error, setError, {
        contact: 'contact',
        contract_amount: 'contract_amount',
        name: 'name',
        payment_amount: 'payment_amount',
        remark: 'remark',
        status: 'status',
      })
      if (!mapped) {
        setError('root', { message: getApiErrorTranslationKey(error) })
      }
    } finally {
      setPending(false)
    }
  })
  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t(
              dynamicI18nKey(
                'customer',
                customer ? 'customer.edit.title' : 'customer.create.title'
              )
            )}
          </DialogTitle>
          <DialogDescription>
            {t(
              dynamicI18nKey(
                'customer',
                customer
                  ? 'customer.edit.description'
                  : 'customer.create.description'
              )
            )}
          </DialogDescription>
        </DialogHeader>
        <form
          className='grid gap-4'
          id='customer-form'
          noValidate
          onSubmit={submit}
        >
          <FormField
            error={
              errors.name?.type === 'server'
                ? errors.name.message
                : errors.name?.message &&
                  t(dynamicI18nKey('customer', errors.name.message))
            }
            htmlFor='customer-name'
            label={t('customer.name')}
            required
          >
            <Input id='customer-name' {...register('name')} />
          </FormField>
          <FormField
            error={
              errors.contact?.type === 'server'
                ? errors.contact.message
                : errors.contact?.message &&
                  t(dynamicI18nKey('customer', errors.contact.message))
            }
            htmlFor='customer-contact'
            label={t('customer.contact')}
          >
            <Input id='customer-contact' {...register('contact')} />
          </FormField>
          <FormField
            error={
              errors.contract_amount?.type === 'server'
                ? errors.contract_amount.message
                : errors.contract_amount?.message &&
                  t(dynamicI18nKey('customer', errors.contract_amount.message))
            }
            htmlFor='customer-contract-amount'
            label={t('customer.contractAmount')}
          >
            <Input
              id='customer-contract-amount'
              inputMode='decimal'
              {...register('contract_amount')}
            />
          </FormField>
          <FormField
            error={
              errors.payment_amount?.type === 'server'
                ? errors.payment_amount.message
                : errors.payment_amount?.message &&
                  t(dynamicI18nKey('customer', errors.payment_amount.message))
            }
            htmlFor='customer-payment-amount'
            label={t('customer.paymentAmount')}
          >
            <Input
              id='customer-payment-amount'
              inputMode='decimal'
              {...register('payment_amount')}
            />
          </FormField>
          <FormField
            htmlFor='customer-status'
            label={t('customer.statusLabel')}
            required
          >
            <Select id='customer-status' {...register('status')}>
              {editableCustomerStatuses.map((status) => (
                <option key={status} value={status}>
                  {t(dynamicI18nKey('customer', `customer.status.${status}`))}
                </option>
              ))}
            </Select>
          </FormField>
          <FormField
            error={
              errors.remark?.type === 'server'
                ? errors.remark.message
                : errors.remark?.message &&
                  t(dynamicI18nKey('customer', errors.remark.message))
            }
            htmlFor='customer-remark'
            label={t('customer.remark')}
          >
            <Textarea
              className='min-h-24'
              id='customer-remark'
              {...register('remark')}
            />
          </FormField>
          {errors.root?.message && (
            <p className='text-destructive text-sm' role='alert'>
              {t(dynamicI18nKey('customer', errors.root.message))}
            </p>
          )}
        </form>
        <DialogFooter>
          <Button onClick={onClose} type='button' variant='outline'>
            {t('common.cancel')}
          </Button>
          <Button disabled={pending} form='customer-form' type='submit'>
            {pending && <Spinner />}
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function CustomerDialogs({
  onClose,
  onRecovery,
  onSaved,
  state,
}: {
  onClose: () => void
  onRecovery: (run: CollectionRunItem, customer: CustomerListItem) => void
  onSaved: () => void
  state: CustomerDialogState
}) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  if (!state) return null
  if (state.action === 'edit' && state.customer.status === 'disabled') {
    return null
  }
  if (state.action === 'create' || state.action === 'edit') {
    return (
      <CustomerFormDialog
        customer={state.action === 'edit' ? state.customer : undefined}
        onClose={onClose}
        onSaved={onSaved}
      />
    )
  }
  const customer = state.customer
  const execute = async () => {
    setPending(true)
    try {
      if (state.action === 'disable') await disableCustomer(customer.id)
      else if (state.action === 'delete') await deleteCustomer(customer.id)
      else {
        const run = await enableCustomer(customer.id)
        onRecovery(run, customer)
      }
      toast.success(
        t(dynamicI18nKey('customer', `customer.toast.${state.action}`))
      )
      onSaved()
      onClose()
    } catch (error) {
      toast.error(
        t(dynamicI18nKey('customer', getApiErrorTranslationKey(error)))
      )
    } finally {
      setPending(false)
    }
  }
  return (
    <ConfirmDialog
      confirmLabel={t(
        dynamicI18nKey('customer', `customer.confirm.${state.action}.action`)
      )}
      description={t(
        dynamicI18nKey(
          'customer',
          `customer.confirm.${state.action}.description`
        ),
        { name: customer.name }
      )}
      onConfirm={() => void execute()}
      onOpenChange={(open) => !open && onClose()}
      open
      pending={pending}
      title={t(
        dynamicI18nKey('customer', `customer.confirm.${state.action}.title`)
      )}
      variant={state.action === 'enable' ? 'primary' : 'destructive'}
    />
  )
}
