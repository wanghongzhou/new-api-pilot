import { zodResolver } from '@hookform/resolvers/zod'
import {
  Delete02Icon,
  Edit03Icon,
  FloppyDiskIcon,
  Refresh01Icon,
  Sent02Icon,
  Undo02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  Controller,
  useForm,
  type FieldErrors,
  type FieldPath,
  type UseFormReturn,
} from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout/section-page-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { FormField } from '@/components/ui/form-field'
import { Input } from '@/components/ui/input'
import { PasswordInput } from '@/components/ui/password-input'
import { Spinner } from '@/components/ui/spinner'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { fromUnixSeconds } from '@/lib/dayjs'
import { applyApiFieldErrors } from '@/lib/form-errors'
import { translateMessageRef } from '@/lib/message-ref'
import { useAuthStore } from '@/stores/auth-store'

import { getSettings, testDingTalkNotification, updateSettings } from '../api'
import {
  buildSettingFieldMap,
  buildSettingPatchItems,
  buildSettingSLOMessageRefs,
  emptySettingsFormValues,
  settingFieldDefinitions,
  settingItemsByKey,
  settingsSecretState,
  settingsSections,
  settingsToFormValues,
  type SettingFieldDefinition,
} from '../contract'
import { settingsKeys } from '../query-keys'
import { createSettingsFormSchema } from '../schema'
import type {
  NotificationTestResult,
  SecretAction,
  SettingItem,
  SettingsFormValues,
} from '../types'

function validationMessage(
  errors: FieldErrors<SettingsFormValues>,
  field: FieldPath<SettingsFormValues>,
  t: (key: string) => string
): string | undefined {
  const error = errors[field as keyof SettingsFormValues]
  if (typeof error?.message !== 'string') return undefined
  if (error.type === 'server') return error.message
  return t(dynamicI18nKey('settings', error.message))
}

function constraint(
  item: SettingItem | undefined,
  key: 'maximum' | 'minimum'
): number | string | undefined {
  const value = item?.constraints[key]
  return typeof value === 'number' || typeof value === 'string'
    ? value
    : undefined
}

function displaySettingValue(
  item: SettingItem | undefined,
  t: (key: string) => string
) {
  if (!item) return t('settings.value.unavailable')
  if (item.secret) {
    return item.configured
      ? item.masked_value || '********'
      : t('settings.value.notConfigured')
  }
  if (item.value_type === 'bool') {
    return item.value === true
      ? t('settings.value.enabled')
      : t('settings.value.disabled')
  }
  if (item.value == null || item.value === '') return t('settings.value.notSet')
  return String(item.value)
}

function UpdatedAt({ item }: { item: SettingItem | undefined }) {
  const { t } = useTranslation()
  if (!item?.updated_at) return null
  return (
    <span className='text-muted-foreground text-xs'>
      {t('settings.updatedAt', {
        time: fromUnixSeconds(item.updated_at).format('YYYY-MM-DD HH:mm:ss'),
      })}
    </span>
  )
}

function ReadonlySetting({ item }: { item: SettingItem | undefined }) {
  const { t } = useTranslation()
  return (
    <div className='grid gap-1.5'>
      <output className='border-border bg-muted/35 flex min-h-10 items-center rounded-md border px-3 text-sm break-all'>
        {displaySettingValue(item, t)}
      </output>
      <UpdatedAt item={item} />
    </div>
  )
}

function EditableSetting({
  definition,
  form,
  item,
}: {
  definition: SettingFieldDefinition
  form: UseFormReturn<SettingsFormValues>
  item: SettingItem | undefined
}) {
  const { t } = useTranslation()
  if (!definition.formName) return <ReadonlySetting item={item} />
  const error = validationMessage(form.formState.errors, definition.formName, t)
  if (definition.kind === 'boolean') {
    return (
      <div className='grid gap-1.5'>
        <label
          className='flex min-h-10 items-center gap-3'
          htmlFor='setting-dingtalk-enabled'
        >
          <Controller
            control={form.control}
            name='dingTalkEnabled'
            render={({ field }) => (
              <Checkbox
                checked={field.value}
                id='setting-dingtalk-enabled'
                onBlur={field.onBlur}
                onCheckedChange={field.onChange}
                ref={field.ref}
              />
            )}
          />
          <span className='text-sm'>
            {form.watch('dingTalkEnabled')
              ? t('settings.value.enabled')
              : t('settings.value.disabled')}
          </span>
        </label>
        {error && (
          <p className='text-destructive text-xs' role='alert'>
            {error}
          </p>
        )}
        <UpdatedAt item={item} />
      </div>
    )
  }
  let inputMode: 'decimal' | 'numeric' | undefined
  if (definition.kind === 'decimal') inputMode = 'decimal'
  else if (definition.kind === 'bigint') inputMode = 'numeric'
  const textInput = (
    <Input
      aria-invalid={Boolean(error)}
      id={`setting-${definition.formName}`}
      inputMode={inputMode}
      max={constraint(item, 'maximum')}
      min={constraint(item, 'minimum')}
      step={definition.step}
      type={definition.kind === 'integer' ? 'number' : 'text'}
      {...form.register(definition.formName)}
    />
  )
  return (
    <FormField
      error={error}
      htmlFor={`setting-${definition.formName}`}
      label={
        <span className='sr-only'>
          {t(dynamicI18nKey('settings', definition.labelKey))}
        </span>
      }
    >
      {textInput}
    </FormField>
  )
}

function SecretSetting({
  actionField,
  form,
  inputField,
  item,
  kind,
}: {
  actionField: 'dingTalkSecretAction' | 'dingTalkWebhookAction'
  form: UseFormReturn<SettingsFormValues>
  inputField: 'dingTalkSecret' | 'dingTalkWebhook'
  item: SettingItem | undefined
  kind: 'secret' | 'webhook'
}) {
  const { t } = useTranslation()
  const [clearConfirmOpen, setClearConfirmOpen] = useState(false)
  const action = form.watch(actionField)
  const error = validationMessage(form.formState.errors, inputField, t)
  const setAction = (next: SecretAction) => {
    form.setValue(actionField, next, {
      shouldDirty: true,
      shouldValidate: true,
    })
    if (next !== 'replace') {
      form.setValue(inputField, '', {
        shouldDirty: true,
        shouldValidate: true,
      })
    }
  }
  const InputControl = kind === 'secret' ? PasswordInput : Input

  return (
    <div className='grid gap-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Badge variant={item?.configured ? 'success' : 'neutral'}>
          {item?.configured
            ? t('settings.secret.configured')
            : t('settings.secret.notConfigured')}
        </Badge>
        {item?.configured && (
          <span
            aria-label={t('settings.secret.masked')}
            className='font-mono text-sm'
          >
            {item.masked_value || '********'}
          </span>
        )}
        {item?.decrypt_error && (
          <Badge variant='destructive'>
            {t('settings.secret.decryptError')}
          </Badge>
        )}
      </div>
      <div
        aria-label={t('settings.secret.action')}
        className='border-border flex w-fit max-w-full flex-wrap rounded-md border p-0.5'
        role='group'
      >
        <Button
          aria-pressed={action === 'keep'}
          disabled={item?.decrypt_error}
          onClick={() => setAction('keep')}
          size='sm'
          type='button'
          variant={action === 'keep' ? 'secondary' : 'ghost'}
        >
          {t('settings.secret.keep')}
        </Button>
        <Button
          aria-pressed={action === 'replace'}
          onClick={() => setAction('replace')}
          size='sm'
          type='button'
          variant={action === 'replace' ? 'secondary' : 'ghost'}
        >
          <HugeiconsIcon icon={Edit03Icon} strokeWidth={2} />
          {t('settings.secret.replace')}
        </Button>
        <Button
          aria-pressed={action === 'clear'}
          onClick={() => setClearConfirmOpen(true)}
          size='sm'
          type='button'
          variant={action === 'clear' ? 'destructive' : 'ghost'}
        >
          <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
          {t('settings.secret.clear')}
        </Button>
      </div>
      {action === 'replace' && (
        <FormField
          error={error}
          htmlFor={`setting-${inputField}`}
          label={t(
            dynamicI18nKey(
              'settings',
              kind === 'secret'
                ? 'settings.secret.newSecret'
                : 'settings.secret.newWebhook'
            )
          )}
          required
        >
          <InputControl
            aria-invalid={Boolean(error)}
            autoComplete='new-password'
            id={`setting-${inputField}`}
            placeholder={
              kind === 'webhook'
                ? t('settings.secret.webhookPlaceholder')
                : undefined
            }
            {...form.register(inputField)}
          />
        </FormField>
      )}
      {action !== 'replace' && error && (
        <p className='text-destructive text-xs' role='alert'>
          {error}
        </p>
      )}
      {action === 'clear' && (
        <p className='text-warning-foreground text-xs' role='status'>
          {t('settings.secret.clearPending')}
        </p>
      )}
      <UpdatedAt item={item} />
      <ConfirmDialog
        confirmLabel={t('settings.secret.confirmClear')}
        description={t('settings.secret.clearDescription')}
        onConfirm={() => {
          setAction('clear')
          setClearConfirmOpen(false)
        }}
        onOpenChange={setClearConfirmOpen}
        open={clearConfirmOpen}
        title={t('settings.secret.clearTitle')}
      />
    </div>
  )
}

function SettingRow({
  definition,
  form,
  isAdmin,
  item,
}: {
  definition: SettingFieldDefinition
  form: UseFormReturn<SettingsFormValues>
  isAdmin: boolean
  item: SettingItem | undefined
}) {
  const { t } = useTranslation()
  let control: ReactNode
  if (!isAdmin || definition.kind === 'readonly') {
    control = <ReadonlySetting item={item} />
  } else if (definition.key === 'notification.dingtalk.webhook') {
    control = (
      <SecretSetting
        actionField='dingTalkWebhookAction'
        form={form}
        inputField='dingTalkWebhook'
        item={item}
        kind='webhook'
      />
    )
  } else if (definition.key === 'notification.dingtalk.secret') {
    control = (
      <SecretSetting
        actionField='dingTalkSecretAction'
        form={form}
        inputField='dingTalkSecret'
        item={item}
        kind='secret'
      />
    )
  } else {
    control = (
      <EditableSetting definition={definition} form={form} item={item} />
    )
  }

  return (
    <div className='grid gap-3 py-4 md:grid-cols-[minmax(0,1fr)_minmax(18rem,32rem)] md:items-start md:gap-8'>
      <div className='min-w-0'>
        <h3 className='text-sm font-medium'>
          {t(dynamicI18nKey('settings', definition.labelKey))}
        </h3>
        <p className='text-muted-foreground mt-1 max-w-2xl text-xs'>
          {t(dynamicI18nKey('settings', definition.descriptionKey))}
        </p>
      </div>
      <div className='min-w-0'>{control}</div>
    </div>
  )
}

function SLOStatus({
  eligible,
  messages,
}: {
  eligible: boolean
  messages: readonly string[]
}) {
  const { t } = useTranslation()
  return (
    <section
      aria-labelledby='settings-slo-title'
      className='border-border flex flex-wrap items-start justify-between gap-4 border-y py-4'
    >
      <div>
        <h2 className='text-sm font-semibold' id='settings-slo-title'>
          {t('settings.slo.title')}
        </h2>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('settings.slo.description')}
        </p>
      </div>
      <div className='grid justify-items-end gap-2 text-right'>
        <Badge variant={eligible ? 'success' : 'warning'}>
          {t(
            dynamicI18nKey(
              'settings',
              eligible ? 'settings.slo.eligible' : 'settings.slo.ineligible'
            )
          )}
        </Badge>
        {messages.map((message) => (
          <p className='text-warning-foreground max-w-xl text-xs' key={message}>
            {message}
          </p>
        ))}
      </div>
    </section>
  )
}

function NotificationTest({
  dirty,
  enabled,
  onResult,
  secretReady,
  webhookReady,
}: {
  dirty: boolean
  enabled: boolean
  onResult: (result: NotificationTestResult) => void
  secretReady: boolean
  webhookReady: boolean
}) {
  const { t } = useTranslation()
  const mutation = useMutation({ mutationFn: testDingTalkNotification })
  const canTest = enabled && secretReady && webhookReady && !dirty
  let disabledReason: string | undefined
  if (dirty) disabledReason = t('settings.notification.saveBeforeTest')
  else if (!enabled) {
    disabledReason = t('settings.notification.enableBeforeTest')
  } else if (!secretReady || !webhookReady) {
    disabledReason = t('settings.notification.configureBeforeTest')
  }

  return (
    <Button
      disabled={!canTest || mutation.isPending}
      onClick={() => {
        mutation.mutate(undefined, {
          onError: (error) =>
            toast.error(
              t(dynamicI18nKey('api', getApiErrorTranslationKey(error)))
            ),
          onSuccess: (result) => {
            onResult(result)
            const message = translateMessageRef(result.message)
            if (result.status === 'success') toast.success(message)
            else if (result.message.code === 'DELIVERY_RETRY_SCHEDULED') {
              toast.warning(message)
            } else toast.error(message)
          },
        })
      }}
      title={disabledReason}
      type='button'
      variant='outline'
    >
      {mutation.isPending ? (
        <Spinner />
      ) : (
        <HugeiconsIcon icon={Sent02Icon} strokeWidth={2} />
      )}
      {t('settings.notification.test')}
    </Button>
  )
}

function notificationResultVariant(
  result: NotificationTestResult
): 'destructive' | 'success' | 'warning' {
  if (result.status === 'success') return 'success'
  if (result.message.code === 'DELIVERY_RETRY_SCHEDULED') return 'warning'
  return 'destructive'
}

export function SettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.user)
  const isAdmin = currentUser?.role === 'admin'
  const settingsQuery = useQuery({
    queryFn: getSettings,
    queryKey: settingsKeys.all,
    staleTime: 30_000,
  })
  const secretState = useMemo(
    () => settingsSecretState(settingsQuery.data),
    [settingsQuery.data]
  )
  const schema = useMemo(
    () => createSettingsFormSchema(secretState),
    [secretState]
  )
  const form = useForm<SettingsFormValues>({
    defaultValues: emptySettingsFormValues,
    mode: 'onBlur',
    resolver: zodResolver(schema),
  })
  const [initialValues, setInitialValues] = useState<SettingsFormValues>(
    emptySettingsFormValues
  )
  const [notificationResult, setNotificationResult] =
    useState<NotificationTestResult | null>(null)
  const updateMutation = useMutation({ mutationFn: updateSettings })
  const formValues = form.watch()

  useEffect(() => {
    if (!settingsQuery.data || form.formState.isDirty) return
    const values = settingsToFormValues(settingsQuery.data)
    setInitialValues(values)
    form.reset(values)
  }, [form, form.formState.isDirty, settingsQuery.data])

  const items = useMemo(
    () => settingItemsByKey(settingsQuery.data),
    [settingsQuery.data]
  )
  const sloMessages = useMemo(
    () =>
      buildSettingSLOMessageRefs(settingsQuery.data, formValues).map(
        (message) => translateMessageRef(message)
      ),
    [formValues, settingsQuery.data]
  )
  const eligible = settingsQuery.data?.[0]?.h15_slo_eligible ?? true
  const dingTalkEnabledItem = items.get('notification.dingtalk.enabled')
  const savedDingTalkEnabled = dingTalkEnabledItem?.value === true
  const save = form.handleSubmit(async (values) => {
    const patchItems = buildSettingPatchItems(values, initialValues)
    if (patchItems.length === 0) {
      form.reset(initialValues)
      return
    }
    try {
      const updated = await updateMutation.mutateAsync({ items: patchItems })
      queryClient.setQueryData(settingsKeys.all, updated)
      const nextValues = settingsToFormValues(updated)
      setInitialValues(nextValues)
      form.reset(nextValues)
      setNotificationResult(null)
      toast.success(t('settings.toast.saved'))
    } catch (error) {
      const mapped = applyApiFieldErrors(
        error,
        form.setError,
        buildSettingFieldMap(patchItems)
      )
      if (!mapped) {
        form.setError('root', {
          message: t(dynamicI18nKey('api', getApiErrorTranslationKey(error))),
          type: 'server',
        })
      }
    }
  })

  const actions = (
    <>
      <Button
        aria-label={t('common.refresh')}
        disabled={settingsQuery.isFetching}
        onClick={() => void settingsQuery.refetch()}
        size='icon'
        title={t('common.refresh')}
        type='button'
        variant='outline'
      >
        {settingsQuery.isFetching ? (
          <Spinner />
        ) : (
          <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
        )}
      </Button>
      {isAdmin && settingsQuery.data && (
        <>
          <Button
            aria-label={t('settings.reset')}
            disabled={!form.formState.isDirty || updateMutation.isPending}
            onClick={() => {
              form.reset(initialValues)
              setNotificationResult(null)
            }}
            size='icon'
            title={t('settings.reset')}
            type='button'
            variant='ghost'
          >
            <HugeiconsIcon icon={Undo02Icon} strokeWidth={2} />
          </Button>
          <Button
            disabled={!form.formState.isDirty || updateMutation.isPending}
            form='settings-form'
            type='submit'
          >
            {updateMutation.isPending ? (
              <Spinner />
            ) : (
              <HugeiconsIcon icon={FloppyDiskIcon} strokeWidth={2} />
            )}
            {t('common.save')}
          </Button>
        </>
      )}
    </>
  )

  return (
    <SectionPageLayout
      actions={actions}
      description={t(
        dynamicI18nKey(
          'settings',
          isAdmin ? 'settings.description.admin' : 'settings.description.viewer'
        )
      )}
      title={t('settings.title')}
    >
      {settingsQuery.isPending && (
        <div className='grid gap-4' role='status'>
          <span className='sr-only'>{t('settings.loading')}</span>
          <div className='bg-muted/50 h-20 animate-pulse rounded-md' />
          <div className='bg-muted/50 h-64 animate-pulse rounded-md' />
        </div>
      )}
      {settingsQuery.isError && (
        <section className='border-destructive/30 bg-destructive/5 border-y p-5'>
          <h2 className='font-medium'>{t('settings.loadError')}</h2>
          <Button
            className='mt-3'
            onClick={() => void settingsQuery.refetch()}
            type='button'
            variant='outline'
          >
            {t('common.retry')}
          </Button>
        </section>
      )}
      {settingsQuery.data && (
        <form className='grid gap-8' id='settings-form' onSubmit={save}>
          <SLOStatus eligible={eligible} messages={sloMessages} />
          {settingsSections.map((section) => (
            <section
              aria-labelledby={`settings-section-${section.key}`}
              className='min-w-0'
              key={section.key}
            >
              <div className='mb-2'>
                <h2
                  className='text-lg font-semibold'
                  id={`settings-section-${section.key}`}
                >
                  {t(dynamicI18nKey('settings', section.titleKey))}
                </h2>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {t(dynamicI18nKey('settings', section.descriptionKey))}
                </p>
              </div>
              <div className='border-border divide-border divide-y border-y'>
                {settingFieldDefinitions
                  .filter((definition) => definition.section === section.key)
                  .map((definition) => (
                    <SettingRow
                      definition={definition}
                      form={form}
                      isAdmin={isAdmin}
                      item={items.get(definition.key)}
                      key={definition.key}
                    />
                  ))}
              </div>
              {section.key === 'notification' && isAdmin && (
                <div className='mt-4 flex flex-wrap items-center gap-3'>
                  <NotificationTest
                    dirty={form.formState.isDirty}
                    enabled={savedDingTalkEnabled}
                    onResult={setNotificationResult}
                    secretReady={
                      secretState.secret.configured &&
                      !secretState.secret.decryptError
                    }
                    webhookReady={
                      secretState.webhook.configured &&
                      !secretState.webhook.decryptError
                    }
                  />
                  {notificationResult && (
                    <div
                      className='flex min-w-0 items-center gap-2'
                      role='status'
                    >
                      <Badge
                        variant={notificationResultVariant(notificationResult)}
                      >
                        {t(
                          dynamicI18nKey(
                            'settings',
                            notificationResult.status === 'success'
                              ? 'settings.notification.testSuccess'
                              : 'settings.notification.testFailed'
                          )
                        )}
                      </Badge>
                      <span className='text-sm break-words'>
                        {translateMessageRef(notificationResult.message)}
                      </span>
                    </div>
                  )}
                </div>
              )}
            </section>
          ))}
          {form.formState.errors.root?.message && (
            <p className='text-destructive text-sm' role='alert'>
              {form.formState.errors.root.message}
            </p>
          )}
        </form>
      )}
    </SectionPageLayout>
  )
}
