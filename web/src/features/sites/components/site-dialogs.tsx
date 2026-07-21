import { zodResolver } from '@hookform/resolvers/zod'
import {
  Alert02Icon,
  CheckmarkCircle02Icon,
  Loading03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey, normalizeApiError } from '@/lib/api'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'
import { applyApiFieldErrors } from '@/lib/form-errors'
import { translateMessageRef } from '@/lib/message-ref'

import {
  backfillSite,
  clearSiteStatisticsEnd,
  deleteSite,
  disableSite,
  enableSite,
  endSiteStatistics,
  getSite,
  preflightSiteBaseUrl,
  probeSite,
  recheckSiteCapabilities,
  refreshSite,
  updateSite,
} from '../api'
import {
  preflightProofInvalidReason,
  publicUrlParts,
  type SitePreflightProof,
} from '../preflight-proof'
import { siteKeys } from '../query-keys'
import {
  normalizeBaseUrl,
  siteBackfillSchema,
  siteFormSchema,
  type SiteFormOutput,
  type SiteFormValues,
} from '../schema'
import type { SiteAuthorizationResult, SiteListItem } from '../types'
import type { SiteAction } from './site-actions'
import { SiteAuthorizationForm } from './site-authorization-form'

export interface SiteDialogState {
  action: SiteAction
  site: SiteListItem
}

interface SiteDialogProps {
  onClose: () => void
  onSaved: (action: SiteAction) => void
  state: SiteDialogState | null
}

function capabilityVisual(status: 'passed' | 'failed' | 'skipped') {
  switch (status) {
    case 'passed':
      return { className: 'text-success', icon: CheckmarkCircle02Icon }
    case 'failed':
      return { className: 'text-destructive', icon: Alert02Icon }
    case 'skipped':
      return { className: 'text-muted-foreground', icon: Loading03Icon }
  }
}

function DetailQueryContent({
  children,
  error,
  pending,
}: {
  children: ReactNode
  error: boolean
  pending: boolean
}) {
  const { t } = useTranslation()
  if (pending) {
    return (
      <div className='flex min-h-32 items-center justify-center'>
        <Spinner />
      </div>
    )
  }
  if (error) {
    return <p className='text-destructive text-sm'>{t('table.loadError')}</p>
  }
  return children
}

function CapabilityResult({ result }: { result: SiteAuthorizationResult }) {
  const { t } = useTranslation()
  return (
    <div className='grid gap-4'>
      <section className='border-success/30 bg-success/5 rounded-lg border p-4'>
        <h3 className='flex items-center gap-2 font-medium'>
          <HugeiconsIcon icon={CheckmarkCircle02Icon} strokeWidth={2} />
          {t('site.proof.passed')}
        </h3>
        <p className='text-muted-foreground mt-2 text-sm'>
          {t('site.proof.summary', {
            rootId: result.root_user_id,
            total: result.first_user_proof.snapshot_total,
          })}
        </p>
      </section>
      <ul className='grid gap-2'>
        {result.capabilities.map((capability) => {
          const visual = capabilityVisual(capability.status)
          return (
            <li
              className='border-border flex items-start gap-2 rounded-md border p-3 text-sm'
              key={capability.key}
            >
              <HugeiconsIcon
                className={visual.className}
                icon={visual.icon}
                strokeWidth={2}
              />
              <span>
                <strong className='block'>
                  {t(
                    dynamicI18nKey('site', `site.capability.${capability.key}`),
                    {
                      defaultValue: capability.key,
                    }
                  )}
                </strong>
                {translateMessageRef(capability.message)}
              </span>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

function EditSiteDialog({
  onClose,
  onSaved,
  site,
}: {
  onClose: () => void
  onSaved: () => void
  site: SiteListItem
}) {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)
  const [preflightProof, setPreflightProof] =
    useState<SitePreflightProof | null>(null)
  const [confirmSameSite, setConfirmSameSite] = useState(false)
  const initialized = useRef(false)
  const detailQuery = useQuery({
    queryFn: () => getSite(site.id),
    queryKey: siteKeys.detail(site.id),
    staleTime: 30_000,
  })
  const {
    clearErrors,
    formState: { errors },
    handleSubmit,
    register,
    reset,
    setError,
    watch,
  } = useForm<SiteFormValues, unknown, SiteFormOutput>({
    defaultValues: { baseUrl: site.base_url, name: site.name, remark: '' },
    resolver: zodResolver(siteFormSchema),
  })
  const candidateUrl = watch('baseUrl')

  useEffect(() => {
    if (!detailQuery.data || initialized.current) return
    initialized.current = true
    reset({
      baseUrl: detailQuery.data.base_url,
      name: detailQuery.data.name,
      remark: detailQuery.data.remark,
    })
  }, [detailQuery.data, reset])

  useEffect(() => {
    if (!preflightProof || !detailQuery.data) return
    const reason = preflightProofInvalidReason(
      preflightProof,
      candidateUrl,
      detailQuery.data.config_version,
      Math.floor(Date.now() / 1000)
    )
    if (!reason) return
    setPreflightProof(null)
    setConfirmSameSite(false)
    if (reason !== 'candidate_changed') {
      setError('root', {
        message: `site.preflight.invalid.${reason}`,
        type: 'validate',
      })
    }
  }, [candidateUrl, detailQuery.data, preflightProof, setError])

  useEffect(() => {
    if (!preflightProof) return
    const expiresAtMilliseconds = preflightProof.result.expires_at * 1000
    const expire = () => {
      setPreflightProof(null)
      setConfirmSameSite(false)
      setError('root', {
        message: 'site.preflight.invalid.expired',
        type: 'validate',
      })
    }
    let timer: number | undefined
    const scheduleExpiration = () => {
      const remaining = expiresAtMilliseconds - Date.now()
      if (remaining <= 0) {
        expire()
        return
      }
      timer = window.setTimeout(
        scheduleExpiration,
        Math.min(remaining, 2_147_000_000)
      )
    }
    scheduleExpiration()
    return () => {
      if (timer != null) window.clearTimeout(timer)
    }
  }, [preflightProof, setError])

  const submit = handleSubmit(async (values) => {
    if (!detailQuery.data) return
    setSubmitting(true)
    try {
      const urlChanged =
        normalizeBaseUrl(values.baseUrl) !==
        normalizeBaseUrl(detailQuery.data.base_url)
      if (urlChanged && !preflightProof) {
        clearErrors('root')
        const result = await preflightSiteBaseUrl(site.id, values.baseUrl)
        setPreflightProof({
          configVersion: detailQuery.data.config_version,
          result,
        })
        setConfirmSameSite(false)
        return
      }
      if (urlChanged && preflightProof) {
        const reason = preflightProofInvalidReason(
          preflightProof,
          values.baseUrl,
          detailQuery.data.config_version,
          Math.floor(Date.now() / 1000)
        )
        if (reason) {
          setPreflightProof(null)
          setConfirmSameSite(false)
          setError('root', {
            message: `site.preflight.invalid.${reason}`,
            type: 'validate',
          })
          return
        }
        if (!confirmSameSite) {
          setError('root', {
            message: 'site.preflight.confirmRequired',
            type: 'validate',
          })
          return
        }
      }
      await updateSite(site.id, {
        base_url: values.baseUrl,
        base_url_preflight_token: urlChanged
          ? preflightProof?.result.preflight_token
          : undefined,
        confirm_same_site: urlChanged ? confirmSameSite : undefined,
        name: values.name,
        remark: values.remark || undefined,
      })
      toast.success(t('site.toast.updated'))
      onSaved()
      onClose()
    } catch (error) {
      const apiError = normalizeApiError(error)
      if (
        apiError.code === 'BASE_URL_PREFLIGHT_REQUIRED' ||
        apiError.code === 'SITE_CONFIG_CHANGED'
      ) {
        setPreflightProof(null)
        setConfirmSameSite(false)
      }
      const mapped = applyApiFieldErrors(error, setError, {
        base_url: 'baseUrl',
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

  const preflight = preflightProof?.result ?? null
  let urlChanged = false
  try {
    urlChanged = Boolean(
      detailQuery.data &&
      normalizeBaseUrl(candidateUrl) !==
        normalizeBaseUrl(detailQuery.data.base_url)
    )
  } catch {
    urlChanged = candidateUrl !== detailQuery.data?.base_url
  }
  const oldUrlParts = preflight
    ? publicUrlParts(preflight.old_public.base_url)
    : null
  const candidateUrlParts = preflight
    ? publicUrlParts(preflight.candidate_public.base_url)
    : null

  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open>
      <DialogContent className='max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('site.edit.title')}</DialogTitle>
          <DialogDescription>{t('site.edit.description')}</DialogDescription>
        </DialogHeader>
        <DetailQueryContent
          error={detailQuery.isError}
          pending={detailQuery.isPending}
        >
          <form
            className='grid gap-4'
            id='edit-site-form'
            noValidate
            onSubmit={submit}
          >
            <FormField
              error={
                errors.name?.message &&
                t(dynamicI18nKey('site', errors.name.message))
              }
              htmlFor='edit-site-name'
              label={t('site.name')}
              required
            >
              <Input id='edit-site-name' {...register('name')} />
            </FormField>
            <FormField
              description={t('site.edit.preflightRequired')}
              error={
                errors.baseUrl?.message &&
                t(dynamicI18nKey('site', errors.baseUrl.message))
              }
              htmlFor='edit-site-url'
              label={t('site.baseUrl')}
              required
            >
              <Input
                id='edit-site-url'
                inputMode='url'
                {...register('baseUrl')}
              />
            </FormField>
            <FormField
              error={
                errors.remark?.message &&
                t(dynamicI18nKey('site', errors.remark.message))
              }
              htmlFor='edit-site-remark'
              label={t('site.remark')}
            >
              <Textarea
                className='min-h-24'
                id='edit-site-remark'
                {...register('remark')}
              />
            </FormField>
            {preflight && (
              <section className='border-border bg-muted/30 rounded-md border p-3 text-sm'>
                <div className='flex flex-wrap items-center justify-between gap-2'>
                  <h3 className='font-medium'>{t('site.preflight.result')}</h3>
                  <Badge
                    variant={
                      preflight.contract_status === 'compatible'
                        ? 'success'
                        : 'destructive'
                    }
                  >
                    {t(
                      dynamicI18nKey(
                        'site',
                        `site.preflight.${preflight.contract_status}`
                      )
                    )}
                  </Badge>
                </div>
                <dl className='mt-3 grid gap-2 sm:grid-cols-3'>
                  <div>
                    <dt className='text-muted-foreground'>
                      {t('site.preflight.changeType')}
                    </dt>
                    <dd>
                      {t(
                        dynamicI18nKey(
                          'site',
                          `site.preflight.change.${preflight.change_type}`
                        )
                      )}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground'>
                      {t('site.preflight.expiresAt')}
                    </dt>
                    <dd>
                      {fromUnixSeconds(preflight.expires_at).format(
                        'YYYY-MM-DD HH:mm:ss'
                      )}
                    </dd>
                  </div>
                </dl>
                <div className='mt-3 overflow-x-auto'>
                  <table className='w-full min-w-136 border-collapse text-left text-xs'>
                    <thead>
                      <tr className='border-border border-b'>
                        <th className='px-2 py-2'>
                          {t('site.preflight.field')}
                        </th>
                        <th className='px-2 py-2'>{t('site.preflight.old')}</th>
                        <th className='px-2 py-2'>
                          {t('site.preflight.candidate')}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr className='border-border border-b'>
                        <th className='px-2 py-2'>
                          {t('site.preflight.origin')}
                        </th>
                        <td className='px-2 py-2 break-all'>
                          {oldUrlParts?.origin}
                        </td>
                        <td className='px-2 py-2 break-all'>
                          {candidateUrlParts?.origin}
                        </td>
                      </tr>
                      <tr className='border-border border-b'>
                        <th className='px-2 py-2'>
                          {t('site.preflight.path')}
                        </th>
                        <td className='px-2 py-2 break-all'>
                          {oldUrlParts?.path}
                        </td>
                        <td className='px-2 py-2 break-all'>
                          {candidateUrlParts?.path}
                        </td>
                      </tr>
                      <tr className='border-border border-b'>
                        <th className='px-2 py-2'>{t('site.systemName')}</th>
                        <td className='px-2 py-2'>
                          {preflight.old_public.system_name}
                        </td>
                        <td className='px-2 py-2'>
                          {preflight.candidate_public.system_name}
                        </td>
                      </tr>
                      <tr>
                        <th className='px-2 py-2'>{t('site.version')}</th>
                        <td className='px-2 py-2'>
                          {preflight.old_public.version}
                        </td>
                        <td className='px-2 py-2'>
                          {preflight.candidate_public.version}
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                {preflight.change_type === 'origin' && (
                  <p className='text-warning-foreground mt-3 font-medium'>
                    {t('site.edit.originChangeWarning')}
                  </p>
                )}
                {preflight.contract_status === 'incompatible' && (
                  <p className='text-destructive mt-3 font-medium'>
                    {t('SITE_INCOMPATIBLE')}
                  </p>
                )}
                {preflight.contract_status === 'compatible' && (
                  <label className='border-border mt-3 flex min-h-12 items-start gap-3 rounded-md border p-3'>
                    <Checkbox
                      checked={confirmSameSite}
                      className='mt-0.5'
                      onCheckedChange={(checked) => {
                        setConfirmSameSite(checked)
                        if (checked) clearErrors('root')
                      }}
                    />
                    <span>{t('site.preflight.confirmSameSite')}</span>
                  </label>
                )}
              </section>
            )}
            {errors.root?.message && (
              <p className='text-destructive text-sm' role='alert'>
                {t(dynamicI18nKey('site', errors.root.message))}
              </p>
            )}
          </form>
        </DetailQueryContent>
        <DialogFooter>
          <Button onClick={onClose} type='button' variant='outline'>
            {t('common.cancel')}
          </Button>
          <Button
            disabled={
              submitting ||
              detailQuery.isPending ||
              preflight?.contract_status === 'incompatible' ||
              (urlChanged && Boolean(preflight) && !confirmSameSite)
            }
            form='edit-site-form'
            type='submit'
          >
            {submitting && <Spinner />}
            {urlChanged && !preflight
              ? t('site.preflight.run')
              : t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function AuthorizationDialog({
  action,
  onClose,
  onSaved,
  site,
}: {
  action: 'authorize' | 'recheck'
  onClose: () => void
  onSaved: () => void
  site: SiteListItem
}) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  const [result, setResult] = useState<SiteAuthorizationResult | null>(null)
  const [errorKey, setErrorKey] = useState<string | null>(null)

  const recheck = async () => {
    setPending(true)
    setErrorKey(null)
    try {
      const next = await recheckSiteCapabilities(site.id)
      setResult(next)
      onSaved()
    } catch (error) {
      setErrorKey(getApiErrorTranslationKey(error))
    } finally {
      setPending(false)
    }
  }

  let content: ReactNode
  if (result) {
    content = <CapabilityResult result={result} />
  } else if (action === 'authorize') {
    content = (
      <SiteAuthorizationForm
        formId='site-reauthorization-form'
        onSuccess={(next) => {
          setResult(next)
          onSaved()
        }}
        siteId={site.id}
        submitLabel={t('site.authorization.verify')}
      />
    )
  } else {
    content = (
      <div className='grid gap-3'>
        <p className='text-sm'>{t('site.recheck.explanation')}</p>
        {errorKey && (
          <p className='text-destructive text-sm' role='alert'>
            {t(dynamicI18nKey('site', errorKey))}
          </p>
        )}
        <Button disabled={pending} onClick={() => void recheck()}>
          {pending && <Spinner />}
          {t('site.recheck.run')}
        </Button>
      </div>
    )
  }

  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open>
      <DialogContent className='max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {t(
              dynamicI18nKey(
                'site',
                action === 'authorize'
                  ? 'site.authorization.title'
                  : 'site.recheck.title'
              )
            )}
          </DialogTitle>
          <DialogDescription>
            {t(
              dynamicI18nKey(
                'site',
                site.management_status === 'disabled'
                  ? 'site.authorization.disabledDescription'
                  : 'site.authorization.description'
              )
            )}
          </DialogDescription>
        </DialogHeader>
        {content}
        {result && (
          <DialogFooter>
            <Button onClick={onClose}>{t('common.done')}</Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}

const backfillFormSchema = z.object({
  end: z.string(),
  onlyMissing: z.boolean(),
  start: z.string(),
})

type BackfillFormValues = z.infer<typeof backfillFormSchema>

function parseOptionalHour(value: string): number | undefined {
  if (!value) return undefined
  const parsed = dayjs.tz(value, 'YYYY-MM-DDTHH:mm', BEIJING_TIMEZONE)
  return parsed.isValid() ? parsed.startOf('hour').unix() : undefined
}

function BackfillDialog({
  onClose,
  onSaved,
  site,
}: {
  onClose: () => void
  onSaved: () => void
  site: SiteListItem
}) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  const {
    control,
    formState: { errors },
    handleSubmit,
    register,
    setError,
  } = useForm<BackfillFormValues>({
    defaultValues: { end: '', onlyMissing: true, start: '' },
    resolver: zodResolver(backfillFormSchema),
  })
  const submit = handleSubmit(async (values) => {
    const request = siteBackfillSchema.safeParse({
      endTimestamp: parseOptionalHour(values.end),
      onlyMissing: values.onlyMissing,
      startTimestamp: parseOptionalHour(values.start),
    })
    if (!request.success) {
      setError('root', {
        message: request.error.issues[0]?.message ?? 'VALIDATION_ERROR',
      })
      return
    }
    setPending(true)
    try {
      await backfillSite(site.id, {
        end_timestamp: request.data.endTimestamp,
        only_missing: request.data.onlyMissing,
        start_timestamp: request.data.startTimestamp,
      })
      toast.success(t('site.toast.backfillQueued'))
      onSaved()
      onClose()
    } catch (error) {
      setError('root', { message: getApiErrorTranslationKey(error) })
    } finally {
      setPending(false)
    }
  })
  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('site.backfill.title')}</DialogTitle>
          <DialogDescription>
            {t('site.backfill.description')}
          </DialogDescription>
        </DialogHeader>
        <form className='grid gap-4' id='site-backfill-form' onSubmit={submit}>
          <FormField htmlFor='backfill-start' label={t('site.backfill.start')}>
            <Input
              id='backfill-start'
              step={3600}
              type='datetime-local'
              {...register('start')}
            />
          </FormField>
          <FormField htmlFor='backfill-end' label={t('site.backfill.end')}>
            <Input
              id='backfill-end'
              step={3600}
              type='datetime-local'
              {...register('end')}
            />
          </FormField>
          <label className='flex min-h-10 items-center gap-2 text-sm'>
            <Controller
              control={control}
              name='onlyMissing'
              render={({ field }) => (
                <Checkbox
                  checked={field.value}
                  onBlur={field.onBlur}
                  onCheckedChange={field.onChange}
                  ref={field.ref}
                />
              )}
            />
            {t('site.backfill.onlyMissing')}
          </label>
          {errors.root?.message && (
            <p className='text-destructive text-sm' role='alert'>
              {t(dynamicI18nKey('site', errors.root.message))}
            </p>
          )}
        </form>
        <DialogFooter>
          <Button onClick={onClose} variant='outline'>
            {t('common.cancel')}
          </Button>
          <Button disabled={pending} form='site-backfill-form' type='submit'>
            {pending && <Spinner />}
            {t('site.backfill.queue')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

const statisticsEndFormSchema = z.object({
  endAt: z.string().min(1, 'site.statisticsEnd.required'),
})

function StatisticsEndDialog({
  onClose,
  onSaved,
  site,
}: {
  onClose: () => void
  onSaved: () => void
  site: SiteListItem
}) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  const {
    formState: { errors },
    handleSubmit,
    register,
    setError,
  } = useForm<z.infer<typeof statisticsEndFormSchema>>({
    defaultValues: { endAt: '' },
    resolver: zodResolver(statisticsEndFormSchema),
  })
  const submit = handleSubmit(async (values) => {
    const timestamp = parseOptionalHour(values.endAt)
    if (timestamp == null) {
      setError('endAt', { message: 'VALIDATION_ERROR' })
      return
    }
    setPending(true)
    try {
      await endSiteStatistics(site.id, timestamp)
      toast.success(t('site.toast.statisticsEnded'))
      onSaved()
      onClose()
    } catch (error) {
      setError('root', { message: getApiErrorTranslationKey(error) })
    } finally {
      setPending(false)
    }
  })
  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('site.statisticsEnd.title')}</DialogTitle>
          <DialogDescription>
            {t('site.statisticsEnd.description')}
          </DialogDescription>
        </DialogHeader>
        <form className='grid gap-4' id='statistics-end-form' onSubmit={submit}>
          <FormField
            description={t('site.statisticsEnd.aligned')}
            error={
              errors.endAt?.message &&
              t(dynamicI18nKey('site', errors.endAt.message))
            }
            htmlFor='statistics-end-at'
            label={t('site.statisticsEnd.time')}
            required
          >
            <Input
              id='statistics-end-at'
              max={
                site.disabled_at == null
                  ? undefined
                  : fromUnixSeconds(site.disabled_at).format('YYYY-MM-DDTHH:00')
              }
              step={3600}
              type='datetime-local'
              {...register('endAt')}
            />
          </FormField>
          {errors.root?.message && (
            <p className='text-destructive text-sm'>
              {t(dynamicI18nKey('site', errors.root.message))}
            </p>
          )}
        </form>
        <DialogFooter>
          <Button onClick={onClose} variant='outline'>
            {t('common.cancel')}
          </Button>
          <Button
            disabled={pending}
            form='statistics-end-form'
            type='submit'
            variant='destructive'
          >
            {pending && <Spinner />}
            {t('site.statisticsEnd.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ManageLifecycleDialog({
  onClose,
  onSaved,
  site,
}: {
  onClose: () => void
  onSaved: (action: SiteAction) => void
  site: SiteListItem
}) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  const [confirmAction, setConfirmAction] = useState<
    'clear-statistics-end' | 'enable' | null
  >(null)
  const [showStatisticsEnd, setShowStatisticsEnd] = useState(false)
  const detailQuery = useQuery({
    queryFn: () => getSite(site.id),
    queryKey: siteKeys.detail(site.id),
    refetchOnMount: 'always',
    staleTime: 0,
  })
  const detail = detailQuery.data

  if (showStatisticsEnd && detail) {
    return (
      <StatisticsEndDialog
        onClose={onClose}
        onSaved={() => onSaved('end-statistics')}
        site={detail}
      />
    )
  }

  const execute = async () => {
    if (!confirmAction) return
    setPending(true)
    try {
      if (confirmAction === 'enable') await enableSite(site.id)
      else await clearSiteStatisticsEnd(site.id)
      toast.success(t(dynamicI18nKey('site', `site.toast.${confirmAction}`)))
      onSaved(confirmAction)
      onClose()
    } catch (error) {
      toast.error(t(dynamicI18nKey('site', getApiErrorTranslationKey(error))))
    } finally {
      setPending(false)
    }
  }

  if (confirmAction) {
    const key =
      confirmAction === 'enable'
        ? 'site.confirm.enable'
        : 'site.confirm.clearStatisticsEnd'
    return (
      <ConfirmDialog
        confirmLabel={t(dynamicI18nKey('site', `${key}.action`))}
        description={t(dynamicI18nKey('site', `${key}.description`), {
          name: detail?.name ?? site.name,
        })}
        onConfirm={() => void execute()}
        onOpenChange={(open) => !open && setConfirmAction(null)}
        open
        pending={pending}
        title={t(dynamicI18nKey('site', `${key}.title`))}
        variant={confirmAction === 'enable' ? 'primary' : 'destructive'}
      />
    )
  }

  const waitingForFreshDetail =
    detailQuery.isPending ||
    detailQuery.isFetching ||
    !detailQuery.isFetchedAfterMount

  let lifecycleContent: ReactNode
  if (waitingForFreshDetail && !detailQuery.isError) {
    lifecycleContent = (
      <div
        aria-label={t('site.lifecycle.loading')}
        className='flex min-h-32 items-center justify-center'
        role='status'
      >
        <Spinner />
      </div>
    )
  } else if (detailQuery.isError || !detail) {
    lifecycleContent = (
      <div className='grid gap-3'>
        <p className='text-destructive text-sm' role='alert'>
          {t('site.lifecycle.loadError')}
        </p>
        <Button onClick={() => void detailQuery.refetch()} variant='outline'>
          {t('common.retry')}
        </Button>
      </div>
    )
  } else if (detail.statistics_end_at != null) {
    lifecycleContent = (
      <section className='border-warning/40 bg-warning/10 grid gap-3 rounded-md border p-4'>
        <div>
          <h3 className='font-medium'>{t('site.lifecycle.endedTitle')}</h3>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('site.lifecycle.endedDescription', {
              time: fromUnixSeconds(detail.statistics_end_at).format(
                'YYYY-MM-DD HH:mm:ss'
              ),
            })}
          </p>
        </div>
        <Button
          onClick={() => setConfirmAction('clear-statistics-end')}
          variant='destructive'
        >
          {t('site.actions.clear-statistics-end')}
        </Button>
      </section>
    )
  } else {
    lifecycleContent = (
      <section className='grid gap-3'>
        <p className='text-muted-foreground text-sm'>
          {t('site.lifecycle.pausedDescription')}
        </p>
        <div className='grid gap-2 sm:grid-cols-2'>
          <Button onClick={() => setConfirmAction('enable')}>
            {t('site.actions.enable')}
          </Button>
          <Button
            onClick={() => setShowStatisticsEnd(true)}
            variant='destructive'
          >
            {t('site.actions.end-statistics')}
          </Button>
        </div>
      </section>
    )
  }

  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('site.lifecycle.title')}</DialogTitle>
          <DialogDescription>
            {t('site.lifecycle.description', { name: site.name })}
          </DialogDescription>
        </DialogHeader>
        {lifecycleContent}
        <DialogFooter>
          <Button onClick={onClose} variant='outline'>
            {t('common.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

const confirmActionKeys = {
  'clear-statistics-end': 'site.confirm.clearStatisticsEnd',
  delete: 'site.confirm.delete',
  disable: 'site.confirm.disable',
  enable: 'site.confirm.enable',
  probe: 'site.confirm.probe',
  refresh: 'site.confirm.refresh',
} as const

type ConfirmAction = keyof typeof confirmActionKeys

function isConfirmAction(action: SiteAction): action is ConfirmAction {
  return action in confirmActionKeys
}

export function SiteDialogs({ onClose, onSaved, state }: SiteDialogProps) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  if (!state) return null

  if (state.action === 'edit') {
    return (
      <EditSiteDialog
        onClose={onClose}
        onSaved={() => onSaved('edit')}
        site={state.site}
      />
    )
  }
  if (state.action === 'authorize' || state.action === 'recheck') {
    return (
      <AuthorizationDialog
        action={state.action}
        onClose={onClose}
        onSaved={() => onSaved(state.action)}
        site={state.site}
      />
    )
  }
  if (state.action === 'backfill') {
    return (
      <BackfillDialog
        onClose={onClose}
        onSaved={() => onSaved('backfill')}
        site={state.site}
      />
    )
  }
  if (state.action === 'manage-lifecycle') {
    return (
      <ManageLifecycleDialog
        onClose={onClose}
        onSaved={onSaved}
        site={state.site}
      />
    )
  }
  if (state.action === 'end-statistics') {
    return (
      <StatisticsEndDialog
        onClose={onClose}
        onSaved={() => onSaved('end-statistics')}
        site={state.site}
      />
    )
  }
  if (!isConfirmAction(state.action)) return null

  const execute = async () => {
    setPending(true)
    try {
      switch (state.action) {
        case 'clear-statistics-end':
          await clearSiteStatisticsEnd(state.site.id)
          break
        case 'delete':
          await deleteSite(state.site.id)
          break
        case 'disable':
          await disableSite(state.site.id)
          break
        case 'enable':
          await enableSite(state.site.id)
          break
        case 'probe':
          await probeSite(state.site.id)
          break
        case 'refresh':
          await refreshSite(state.site.id)
          break
      }
      toast.success(t(dynamicI18nKey('site', `site.toast.${state.action}`)))
      onSaved(state.action)
      onClose()
    } catch (error) {
      toast.error(t(dynamicI18nKey('site', getApiErrorTranslationKey(error))))
    } finally {
      setPending(false)
    }
  }

  return (
    <ConfirmDialog
      confirmLabel={t(
        dynamicI18nKey('site', `${confirmActionKeys[state.action]}.action`)
      )}
      description={t(
        dynamicI18nKey(
          'site',
          `${confirmActionKeys[state.action]}.description`
        ),
        {
          name: state.site.name,
        }
      )}
      onConfirm={() => void execute()}
      onOpenChange={(open) => !open && onClose()}
      open
      pending={pending}
      title={t(
        dynamicI18nKey('site', `${confirmActionKeys[state.action]}.title`)
      )}
      variant={
        state.action === 'enable' ||
        state.action === 'probe' ||
        state.action === 'refresh'
          ? 'primary'
          : 'destructive'
      }
    />
  )
}
