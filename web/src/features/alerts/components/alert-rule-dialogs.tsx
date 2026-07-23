import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { useMemo } from 'react'
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
import { Spinner } from '@/components/ui/spinner'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import type { IdString } from '@/lib/api-types'
import { applyApiFieldErrors } from '@/lib/form-errors'

import {
  createAlertRuleOverride,
  deleteAlertRuleOverride,
  updateAlertRule,
} from '../api'
import {
  alertRuleFormValues,
  alertRuleOverrideRequest,
  alertRuleUpdateRequest,
  hasAlertRuleChanges,
  pairedAlertRule,
} from '../contract'
import { createAlertRuleFormSchema, type AlertRuleFormOutput } from '../schema'
import type { AlertRuleFormValues, AlertRuleItem } from '../types'
import {
  alertLevelText,
  alertRuleDescription,
  alertRuleName,
  ruleScopeText,
} from './alert-ui'

function formError(
  error: { message?: unknown; type?: string } | undefined,
  t: (key: string) => string
): string | undefined {
  if (typeof error?.message !== 'string') return undefined
  if (error.type === 'server') return error.message
  return t(dynamicI18nKey('alert', error.message))
}

export function AlertRuleFormDialog({
  onClose,
  onSaved,
  rule,
  rules,
  siteId,
}: {
  onClose: () => void
  onSaved: () => void
  rule: AlertRuleItem
  rules: readonly AlertRuleItem[]
  siteId?: IdString
}) {
  const { t } = useTranslation()
  const createOverride = rule.inherited && Boolean(siteId)
  const paired = pairedAlertRule(rules, rule)
  const schema = useMemo(
    () => createAlertRuleFormSchema(rule, paired),
    [paired, rule]
  )
  const initialValues = useMemo(() => alertRuleFormValues(rule), [rule])
  const mutation = useMutation({
    mutationFn: async (values: AlertRuleFormOutput) => {
      if (rule.inherited) {
        if (!siteId) throw new Error()
        return createAlertRuleOverride(
          alertRuleOverrideRequest(values, rule, siteId)
        )
      }
      const request = alertRuleUpdateRequest(values, initialValues, rule)
      if (!hasAlertRuleChanges(request)) return rule
      return updateAlertRule(rule.effective_rule_id, request)
    },
  })
  const {
    control,
    formState: { errors, isDirty },
    handleSubmit,
    register,
    setError,
  } = useForm<AlertRuleFormValues, unknown, AlertRuleFormOutput>({
    defaultValues: initialValues,
    resolver: zodResolver(schema),
  })
  const submit = handleSubmit(async (values) => {
    try {
      await mutation.mutateAsync(values)
      toast.success(
        createOverride
          ? t('alerts.rules.toast.overrideCreated')
          : t('alerts.rules.toast.updated')
      )
      onSaved()
      onClose()
    } catch (error) {
      const mapped = applyApiFieldErrors(error, setError, {
        body: 'root',
        enabled: 'enabled',
        for_times: 'forTimes',
        threshold_value: 'thresholdValue',
      })
      if (!mapped) {
        setError('root', {
          message: getApiErrorTranslationKey(error),
          type: 'server-root',
        })
      }
    }
  })
  return (
    <Drawer direction='right' onOpenChange={(open) => !open && onClose()} open>
      <DrawerContent className={sideDrawerContentClassName('sm:max-w-xl')}>
        <DrawerHeader className={sideDrawerHeaderClassName()}>
          <DrawerTitle>
            {createOverride
              ? t('alerts.rules.createOverride')
              : t('alerts.rules.edit')}
          </DrawerTitle>
          <DrawerDescription>
            {createOverride
              ? t('alerts.rules.createOverrideDescription')
              : t('alerts.rules.editDescription')}
          </DrawerDescription>
        </DrawerHeader>
        <form
          className={sideDrawerFormClassName('gap-4')}
          id='alert-rule-form'
          noValidate
          onSubmit={submit}
        >
          <dl className='border-border divide-border grid divide-y border-y text-sm'>
            <div className='grid gap-1 py-2 sm:grid-cols-[9rem_1fr]'>
              <dt className='text-muted-foreground'>
                {t('alerts.table.rule')}
              </dt>
              <dd>
                <p>{alertRuleName(t, rule.rule_key)}</p>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {alertRuleDescription(t, rule.rule_key)}
                </p>
              </dd>
            </div>
            <div className='grid gap-1 py-2 sm:grid-cols-[9rem_1fr]'>
              <dt className='text-muted-foreground'>
                {t('alerts.rules.level')}
              </dt>
              <dd>{alertLevelText(t, rule.level)}</dd>
            </div>
            <div className='grid gap-1 py-2 sm:grid-cols-[9rem_1fr]'>
              <dt className='text-muted-foreground'>
                {t('alerts.rules.metric')}
              </dt>
              <dd className='font-mono text-xs break-all'>{rule.metric}</dd>
            </div>
            <div className='grid gap-1 py-2 sm:grid-cols-[9rem_1fr]'>
              <dt className='text-muted-foreground'>
                {t('alerts.rules.compareOperator')}
              </dt>
              <dd className='font-mono'>{rule.compare_operator}</dd>
            </div>
            <div className='grid gap-1 py-2 sm:grid-cols-[9rem_1fr]'>
              <dt className='text-muted-foreground'>
                {t('alerts.rules.scope')}
              </dt>
              <dd>
                {createOverride
                  ? t('alerts.rules.scope.site')
                  : ruleScopeText(t, rule.scope_type)}
              </dd>
            </div>
          </dl>
          <FormField
            error={formError(errors.enabled, t)}
            htmlFor='alert-rule-enabled'
            label={t('alerts.rules.enabled')}
          >
            <label
              className='flex min-h-10 items-center gap-3'
              htmlFor='alert-rule-enabled'
            >
              <Controller
                control={control}
                name='enabled'
                render={({ field }) => (
                  <Checkbox
                    checked={field.value}
                    id='alert-rule-enabled'
                    onBlur={field.onBlur}
                    onCheckedChange={field.onChange}
                    ref={field.ref}
                  />
                )}
              />
              <span className='text-sm'>
                {t('alerts.rules.enabledDescription')}
              </span>
            </label>
          </FormField>
          {rule.constraints.threshold_editable ? (
            <FormField
              description={t('alerts.rules.thresholdDescription', {
                maximum:
                  rule.constraints.threshold_max ??
                  t('alerts.value.unavailable'),
                minimum:
                  rule.constraints.threshold_min ??
                  t('alerts.value.unavailable'),
              })}
              error={formError(errors.thresholdValue, t)}
              htmlFor='alert-rule-threshold'
              label={t('alerts.rules.threshold')}
              required
            >
              <Input
                id='alert-rule-threshold'
                inputMode='decimal'
                {...register('thresholdValue')}
              />
            </FormField>
          ) : (
            <div className='grid gap-1.5'>
              <span className='text-sm font-medium'>
                {t('alerts.rules.threshold')}
              </span>
              <output className='border-border bg-muted/35 flex min-h-10 items-center rounded-md border px-3 text-sm'>
                {t('alerts.rules.fixedBySystem')}
              </output>
            </div>
          )}
          {rule.constraints.for_times_editable ? (
            <FormField
              description={t('alerts.rules.forTimesDescription', {
                maximum: rule.constraints.for_times_max,
                minimum: rule.constraints.for_times_min,
              })}
              error={formError(errors.forTimes, t)}
              htmlFor='alert-rule-for-times'
              label={t('alerts.rules.forTimes')}
              required
            >
              <Input
                id='alert-rule-for-times'
                inputMode='numeric'
                max={rule.constraints.for_times_max}
                min={rule.constraints.for_times_min}
                type='number'
                {...register('forTimes')}
              />
            </FormField>
          ) : (
            <div className='grid gap-1.5'>
              <span className='text-sm font-medium'>
                {t('alerts.rules.forTimes')}
              </span>
              <output className='border-border bg-muted/35 flex min-h-10 items-center rounded-md border px-3 text-sm'>
                {rule.for_times}
              </output>
            </div>
          )}
          {errors.root?.message && (
            <p className='text-destructive text-sm' role='alert'>
              {errors.root.type === 'server'
                ? errors.root.message
                : t(dynamicI18nKey('api', String(errors.root.message)))}
            </p>
          )}
        </form>
        <DrawerFooter className={sideDrawerFooterClassName()}>
          <Button onClick={onClose} type='button' variant='outline'>
            {t('common.cancel')}
          </Button>
          <Button
            disabled={mutation.isPending || (!createOverride && !isDirty)}
            form='alert-rule-form'
            type='submit'
          >
            {mutation.isPending && <Spinner />}
            {createOverride
              ? t('alerts.rules.createOverride')
              : t('common.save')}
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  )
}

export function AlertRuleResetDialog({
  onClose,
  onSaved,
  rule,
}: {
  onClose: () => void
  onSaved: () => void
  rule: AlertRuleItem
}) {
  const { t } = useTranslation()
  const mutation = useMutation({
    mutationFn: () => {
      if (!rule.override_rule_id) throw new Error()
      return deleteAlertRuleOverride(rule.override_rule_id)
    },
    onError: (error) =>
      toast.error(t(dynamicI18nKey('api', getApiErrorTranslationKey(error)))),
    onSuccess: () => {
      toast.success(t('alerts.rules.toast.restored'))
      onSaved()
      onClose()
    },
  })
  return (
    <ConfirmDialog
      confirmLabel={t('alerts.rules.restoreGlobal')}
      description={t('alerts.rules.restoreGlobalDescription', {
        name: alertRuleName(t, rule.rule_key),
      })}
      onConfirm={() => mutation.mutate()}
      onOpenChange={(open) => !open && onClose()}
      open
      pending={mutation.isPending}
      title={t('alerts.rules.restoreGlobalTitle')}
    />
  )
}
