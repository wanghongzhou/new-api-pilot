import { zodResolver } from '@hookform/resolvers/zod'
import {
  Alert02Icon,
  CheckmarkCircle02Icon,
  Search01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState, type CompositionEvent } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
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
import { SelectControl as Select } from '@/components/ui/select-control'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { listCustomers } from '@/features/customers/api'
import { customerKeys } from '@/features/customers/query-keys'
import type { CustomerListParams } from '@/features/customers/types'
import { listSites } from '@/features/sites/api'
import { siteKeys } from '@/features/sites/query-keys'
import type { SiteListParams } from '@/features/sites/types'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { parseIdString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import { applyApiFieldErrors } from '@/lib/form-errors'
import { useAuthStore } from '@/stores/auth-store'

import { createAccount, listRemoteUsers } from '../api'
import { accountKeys } from '../query-keys'
import { findExactRemoteUser, remoteUserChanged } from '../remote-review'
import {
  accountCustomerStepSchema,
  accountOnboardingSchema,
  accountRemoteUserStepSchema,
  type AccountOnboardingValues,
} from '../schema'
import type { AccountDetail, RemoteUserItem } from '../types'

const steps = ['customer', 'remoteUser', 'review', 'binding'] as const

function RemoteUserSummary({ user }: { user: RemoteUserItem }) {
  const { t } = useTranslation()
  return (
    <dl className='grid gap-3 text-sm sm:grid-cols-2'>
      <div>
        <dt className='text-muted-foreground'>{t('account.remoteUserId')}</dt>
        <dd>{user.id}</dd>
      </div>
      <div>
        <dt className='text-muted-foreground'>{t('account.username')}</dt>
        <dd>{user.username}</dd>
      </div>
      <div>
        <dt className='text-muted-foreground'>{t('account.displayName')}</dt>
        <dd>{user.display_name || '-'}</dd>
      </div>
      <div>
        <dt className='text-muted-foreground'>{t('account.remoteGroup')}</dt>
        <dd>{user.group || '-'}</dd>
      </div>
      <div>
        <dt className='text-muted-foreground'>
          {t('account.remoteStatusLabel')}
        </dt>
        <dd>
          {t(
            dynamicI18nKey(
              'account',
              user.status === 1
                ? 'account.remoteStatus.enabled'
                : 'account.remoteStatus.disabled'
            )
          )}
        </dd>
      </div>
      <div>
        <dt className='text-muted-foreground'>
          {t('account.remoteCreatedAt')}
        </dt>
        <dd>
          {fromUnixSeconds(user.created_at).format('YYYY-MM-DD HH:mm:ss')}
        </dd>
      </div>
    </dl>
  )
}

export function AccountOnboardingDrawer({
  initialCustomerId,
  onComplete,
  onOpenChange,
  open,
}: {
  initialCustomerId?: string
  onComplete: (account: AccountDetail) => void
  onOpenChange: (open: boolean) => void
  open: boolean
}) {
  const { t } = useTranslation()
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')
  const [step, setStep] = useState(0)
  const [keyword, setKeyword] = useState('')
  const [debouncedKeyword, setDebouncedKeyword] = useState('')
  const [composing, setComposing] = useState(false)
  const [selectedUser, setSelectedUser] = useState<RemoteUserItem | null>(null)
  const [reviewedUser, setReviewedUser] = useState<RemoteUserItem | null>(null)
  const [reviewing, setReviewing] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [reviewError, setReviewError] = useState<string | null>(null)
  const [discardConfirmOpen, setDiscardConfirmOpen] = useState(false)
  const {
    control,
    formState: { errors, isDirty },
    getValues,
    register,
    reset,
    setError,
    setValue,
    trigger,
    watch,
  } = useForm<AccountOnboardingValues>({
    defaultValues: {
      bindingConfirmed: false,
      customerId: initialCustomerId ?? '',
      remark: '',
      remoteUserId: '',
      siteId: '',
    },
    resolver: zodResolver(accountOnboardingSchema),
  })
  const customerId = watch('customerId')
  const siteId = watch('siteId')
  const bindingConfirmed = watch('bindingConfirmed')

  useEffect(() => {
    if (!open) return
    setStep(0)
    setKeyword('')
    setDebouncedKeyword('')
    setSelectedUser(null)
    setReviewedUser(null)
    setReviewError(null)
    reset({
      bindingConfirmed: false,
      customerId: initialCustomerId ?? '',
      remark: '',
      remoteUserId: '',
      siteId: '',
    })
  }, [initialCustomerId, open, reset])

  useEffect(() => {
    if (composing) return
    const timer = window.setTimeout(
      () => setDebouncedKeyword(keyword.trim()),
      500
    )
    return () => window.clearTimeout(timer)
  }, [composing, keyword])

  const customerParams = useMemo<CustomerListParams>(
    () => ({
      p: 1,
      page_size: 100,
      sort_by: 'name',
      sort_order: 'asc',
      status: ['using'],
    }),
    []
  )
  const customersQuery = useQuery({
    enabled: open && Boolean(isAdmin),
    queryFn: () => listCustomers(customerParams),
    queryKey: customerKeys.list(customerParams),
    staleTime: 5 * 60_000,
  })
  const siteParams = useMemo<SiteListParams>(
    () => ({
      auth_status: ['authorized'],
      management_status: ['active'],
      p: 1,
      page_size: 100,
      sort_by: 'name',
      sort_order: 'asc' as const,
    }),
    []
  )
  const sitesQuery = useQuery({
    enabled: open && Boolean(isAdmin),
    queryFn: () => listSites(siteParams),
    queryKey: siteKeys.list(siteParams),
    staleTime: 5 * 60_000,
  })
  const eligibleSites = (sitesQuery.data?.items ?? []).filter(
    (site) =>
      site.management_status === 'active' &&
      site.auth_status === 'authorized' &&
      site.data_export_enabled === true
  )
  const remoteParams = useMemo(
    () => ({ keyword: debouncedKeyword, p: 1, page_size: 100 }),
    [debouncedKeyword]
  )
  const remoteUsersQuery = useQuery({
    enabled:
      open &&
      Boolean(isAdmin) &&
      step === 1 &&
      /^[1-9]\d*$/.test(siteId) &&
      debouncedKeyword.length > 0,
    queryFn: () => listRemoteUsers(parseIdString(siteId), remoteParams),
    queryKey: accountKeys.remoteUsers(siteId, remoteParams),
    staleTime: 0,
  })

  const preciseReview = async (): Promise<RemoteUserItem | null> => {
    if (!selectedUser || !/^[1-9]\d*$/.test(siteId)) return null
    setReviewing(true)
    setReviewError(null)
    try {
      const page = await listRemoteUsers(parseIdString(siteId), {
        keyword: selectedUser.id,
        p: 1,
        page_size: 100,
      })
      const exact = findExactRemoteUser(page, selectedUser.id)
      if (!exact) {
        setReviewError('account.onboarding.reviewMissing')
        return null
      }
      if (exact.already_managed) {
        setReviewError('account.onboarding.reviewManaged')
        return null
      }
      if (remoteUserChanged(selectedUser, exact)) {
        setSelectedUser(exact)
        setReviewedUser(exact)
        setValue('remoteUserId', exact.id)
        setReviewError('account.onboarding.reviewDrift')
        return null
      }
      setReviewedUser(exact)
      return exact
    } catch (error) {
      setReviewError(getApiErrorTranslationKey(error))
      return null
    } finally {
      setReviewing(false)
    }
  }

  const next = async () => {
    if (step === 0) {
      const values = accountCustomerStepSchema.safeParse(getValues())
      if (!values.success) {
        await trigger('customerId')
        return
      }
      setStep(1)
      return
    }
    if (step === 1) {
      const values = accountRemoteUserStepSchema.safeParse(getValues())
      if (!values.success || !selectedUser) {
        await trigger(['siteId', 'remoteUserId'])
        if (!selectedUser) setReviewError('account.onboarding.selectRemoteUser')
        return
      }
      const reviewed = await preciseReview()
      if (reviewed) setStep(2)
      return
    }
    if (step === 2) setStep(3)
  }

  const submit = async () => {
    const parsed = accountOnboardingSchema.safeParse(getValues())
    if (!parsed.success || !bindingConfirmed) {
      await trigger()
      return
    }
    const previousReviewed = reviewedUser
    const exact = await preciseReview()
    if (
      !exact ||
      !previousReviewed ||
      remoteUserChanged(previousReviewed, exact)
    ) {
      if (
        exact &&
        previousReviewed &&
        remoteUserChanged(previousReviewed, exact)
      ) {
        setReviewError('account.onboarding.reviewDrift')
      }
      setStep(2)
      return
    }
    setSubmitting(true)
    try {
      const account = await createAccount({
        customer_id: parseIdString(parsed.data.customerId),
        remote_user_id: parseIdString(parsed.data.remoteUserId),
        remark: parsed.data.remark || undefined,
        site_id: parseIdString(parsed.data.siteId),
      })
      toast.success(t('account.toast.created'))
      onComplete(account)
      onOpenChange(false)
    } catch (error) {
      const mapped = applyApiFieldErrors(error, setError, {
        customer_id: 'customerId',
        remote_user_id: 'remoteUserId',
        remark: 'remark',
        site_id: 'siteId',
      })
      if (!mapped) {
        setError('root', { message: getApiErrorTranslationKey(error) })
      }
    } finally {
      setSubmitting(false)
    }
  }

  const requestClose = () => {
    if (isDirty || keyword.trim() || selectedUser || step > 0) {
      setDiscardConfirmOpen(true)
      return
    }
    onOpenChange(false)
  }

  if (!isAdmin) return null
  return (
    <>
      <Drawer
        onOpenChange={(nextOpen) => {
          if (nextOpen) onOpenChange(true)
          else requestClose()
        }}
        open={open}
        direction='right'
      >
        <DrawerContent className='data-[vaul-drawer-direction=right]:sm:max-w-3xl'>
          <DrawerHeader>
            <DrawerTitle>{t('account.onboarding.title')}</DrawerTitle>
            <DrawerDescription>
              {t('account.onboarding.description')}
            </DrawerDescription>
          </DrawerHeader>
          <ol
            aria-label={t('account.onboarding.progress')}
            className='my-6 grid grid-cols-4 gap-2'
          >
            {steps.map((item, index) => (
              <li
                aria-current={index === step ? 'step' : undefined}
                className='min-w-0 text-center'
                key={item}
              >
                <span
                  className={
                    index <= step
                      ? 'bg-primary text-primary-foreground mx-auto flex size-8 items-center justify-center rounded-full text-sm'
                      : 'bg-muted text-muted-foreground mx-auto flex size-8 items-center justify-center rounded-full text-sm'
                  }
                >
                  {index + 1}
                </span>
                <span className='mt-1 block text-xs'>
                  {t(
                    dynamicI18nKey('account', `account.onboarding.step.${item}`)
                  )}
                </span>
              </li>
            ))}
          </ol>

          {step === 0 && (
            <div className='grid gap-4'>
              <FormField
                description={t('account.onboarding.customerDescription')}
                error={
                  errors.customerId?.type === 'server'
                    ? errors.customerId.message
                    : errors.customerId?.message &&
                      t(dynamicI18nKey('account', errors.customerId.message))
                }
                htmlFor='account-onboarding-customer'
                label={t('account.customer')}
                required
              >
                <Select
                  id='account-onboarding-customer'
                  name='customerId'
                  onChange={(event) =>
                    setValue('customerId', event.target.value, {
                      shouldDirty: true,
                      shouldValidate: true,
                    })
                  }
                  portalled={false}
                  value={customerId}
                >
                  <option value=''>
                    {t('account.onboarding.chooseCustomer')}
                  </option>
                  {(customersQuery.data?.items ?? []).map((customer) => (
                    <option key={customer.id} value={customer.id}>
                      {customer.name}
                    </option>
                  ))}
                </Select>
              </FormField>
              {customersQuery.isPending && <Spinner />}
              {customersQuery.isError && (
                <p className='text-destructive text-sm'>
                  {t('customer.listLoadError')}
                </p>
              )}
            </div>
          )}

          {step === 1 && (
            <div className='grid gap-4'>
              <FormField
                description={t('account.onboarding.siteDescription')}
                error={
                  errors.siteId?.type === 'server'
                    ? errors.siteId.message
                    : errors.siteId?.message &&
                      t(dynamicI18nKey('account', errors.siteId.message))
                }
                htmlFor='account-onboarding-site'
                label={t('account.site')}
                required
              >
                <Select
                  id='account-onboarding-site'
                  name='siteId'
                  onChange={(event) => {
                    setValue('siteId', event.target.value, {
                      shouldDirty: true,
                      shouldValidate: true,
                    })
                    setSelectedUser(null)
                    setReviewedUser(null)
                    setValue('remoteUserId', '')
                    setReviewError(null)
                  }}
                  portalled={false}
                  value={siteId}
                >
                  <option value=''>{t('account.onboarding.chooseSite')}</option>
                  {eligibleSites.map((site) => (
                    <option key={site.id} value={site.id}>
                      {site.name}
                    </option>
                  ))}
                </Select>
              </FormField>
              <FormField
                description={t('account.onboarding.searchDescription')}
                htmlFor='account-remote-search'
                label={t('account.onboarding.searchRemoteUser')}
              >
                <div className='relative'>
                  <HugeiconsIcon
                    className='text-muted-foreground absolute top-3 left-3'
                    icon={Search01Icon}
                    size={18}
                    strokeWidth={2}
                  />
                  <Input
                    className='pl-10'
                    disabled={!siteId}
                    id='account-remote-search'
                    onChange={(event) => {
                      setKeyword(event.target.value)
                      setSelectedUser(null)
                      setReviewedUser(null)
                      setValue('remoteUserId', '')
                    }}
                    onCompositionEnd={(
                      event: CompositionEvent<HTMLInputElement>
                    ) => {
                      setComposing(false)
                      setKeyword(event.currentTarget.value)
                    }}
                    onCompositionStart={() => setComposing(true)}
                    placeholder={t('account.onboarding.searchPlaceholder')}
                    value={keyword}
                  />
                </div>
              </FormField>
              {remoteUsersQuery.isFetching && (
                <div className='flex items-center gap-2 text-sm'>
                  <Spinner />
                  {t('account.onboarding.searching')}
                </div>
              )}
              {remoteUsersQuery.isError && (
                <p className='text-destructive text-sm'>
                  {t('account.onboarding.searchError')}
                </p>
              )}
              <div className='grid gap-2'>
                {(remoteUsersQuery.data?.items ?? []).map((user) => (
                  <button
                    aria-pressed={selectedUser?.id === user.id}
                    className='border-border hover:bg-muted grid min-h-16 gap-1 rounded-md border p-3 text-left text-sm disabled:opacity-60'
                    disabled={user.already_managed}
                    key={user.id}
                    onClick={() => {
                      setSelectedUser(user)
                      setReviewedUser(null)
                      setValue('remoteUserId', user.id, {
                        shouldValidate: true,
                      })
                      setReviewError(null)
                    }}
                    type='button'
                  >
                    <span className='flex flex-wrap items-center justify-between gap-2'>
                      <strong>
                        {user.username}{' '}
                        <span className='text-muted-foreground font-normal'>
                          #{user.id}
                        </span>
                      </strong>
                      {user.already_managed && (
                        <Badge variant='warning'>
                          {t('account.onboarding.alreadyManaged', {
                            customer: user.managed_customer_name,
                          })}
                        </Badge>
                      )}
                    </span>
                    <span className='text-muted-foreground'>
                      {t('account.onboarding.remoteUserSummary', {
                        displayName: user.display_name || '-',
                        group: user.group || '-',
                      })}
                    </span>
                  </button>
                ))}
              </div>
              <input type='hidden' {...register('remoteUserId')} />
              {errors.remoteUserId?.message && (
                <p className='text-destructive text-sm'>
                  {errors.remoteUserId.type === 'server'
                    ? errors.remoteUserId.message
                    : t(dynamicI18nKey('account', errors.remoteUserId.message))}
                </p>
              )}
            </div>
          )}

          {step === 2 && reviewedUser && (
            <section className='border-border bg-muted/30 grid gap-4 rounded-md border p-4'>
              <div className='flex items-center gap-2'>
                <HugeiconsIcon
                  className='text-success'
                  icon={CheckmarkCircle02Icon}
                  strokeWidth={2}
                />
                <h2 className='font-medium'>
                  {t('account.onboarding.reviewTitle')}
                </h2>
              </div>
              <RemoteUserSummary user={reviewedUser} />
              <p className='text-muted-foreground text-sm'>
                {t('account.onboarding.reviewDescription')}
              </p>
            </section>
          )}

          {step === 3 && reviewedUser && (
            <div className='grid gap-4'>
              <section className='border-warning/40 bg-warning/10 grid gap-3 rounded-md border p-4'>
                <div className='flex items-center gap-2'>
                  <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
                  <h2 className='font-medium'>
                    {t('account.onboarding.bindingTitle')}
                  </h2>
                </div>
                <dl className='grid gap-2 text-sm sm:grid-cols-3'>
                  <div>
                    <dt className='text-muted-foreground'>
                      {t('account.customer')}
                    </dt>
                    <dd>
                      {
                        customersQuery.data?.items.find(
                          (item) => item.id === customerId
                        )?.name
                      }
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground'>
                      {t('account.site')}
                    </dt>
                    <dd>
                      {eligibleSites.find((item) => item.id === siteId)?.name}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground'>
                      {t('account.remoteUserId')}
                    </dt>
                    <dd>{reviewedUser.id}</dd>
                  </div>
                </dl>
                <p className='text-sm'>
                  {t('account.onboarding.bindingImmutable')}
                </p>
              </section>
              <FormField
                error={
                  errors.remark?.type === 'server'
                    ? errors.remark.message
                    : errors.remark?.message &&
                      t(dynamicI18nKey('account', errors.remark.message))
                }
                htmlFor='account-onboarding-remark'
                label={t('account.remark')}
              >
                <Textarea
                  className='min-h-24'
                  id='account-onboarding-remark'
                  {...register('remark')}
                />
              </FormField>
              <label className='border-border flex min-h-12 items-start gap-3 rounded-md border p-3 text-sm'>
                <Controller
                  control={control}
                  name='bindingConfirmed'
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
                <span>{t('account.onboarding.bindingConfirm')}</span>
              </label>
              {errors.bindingConfirmed?.message && (
                <p className='text-destructive text-sm'>
                  {t(
                    dynamicI18nKey('account', errors.bindingConfirmed.message)
                  )}
                </p>
              )}
            </div>
          )}

          {reviewError && (
            <p className='text-destructive mt-4 text-sm' role='alert'>
              {t(dynamicI18nKey('account', reviewError))}
            </p>
          )}
          {errors.root?.message && (
            <p className='text-destructive mt-4 text-sm' role='alert'>
              {t(dynamicI18nKey('account', errors.root.message))}
            </p>
          )}

          <DrawerFooter>
            <Button onClick={requestClose} variant='outline'>
              {t('common.cancel')}
            </Button>
            {step > 0 && (
              <Button
                onClick={() => {
                  setStep((current) => current - 1)
                  setReviewError(null)
                }}
                variant='outline'
              >
                {t('common.back')}
              </Button>
            )}
            {step < 3 ? (
              <Button disabled={reviewing} onClick={() => void next()}>
                {reviewing && <Spinner />}
                {t('common.continue')}
              </Button>
            ) : (
              <Button
                disabled={submitting || reviewing || !bindingConfirmed}
                onClick={() => void submit()}
              >
                {(submitting || reviewing) && <Spinner />}
                {t('account.onboarding.create')}
              </Button>
            )}
          </DrawerFooter>
        </DrawerContent>
      </Drawer>
      <ConfirmDialog
        confirmLabel={t('account.onboarding.discardAction')}
        description={t('account.onboarding.discardDescription')}
        onConfirm={() => {
          setDiscardConfirmOpen(false)
          reset()
          onOpenChange(false)
        }}
        onOpenChange={setDiscardConfirmOpen}
        open={discardConfirmOpen}
        title={t('account.onboarding.discardTitle')}
      />
    </>
  )
}
