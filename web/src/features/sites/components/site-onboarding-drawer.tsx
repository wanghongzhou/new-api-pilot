import { zodResolver } from '@hookform/resolvers/zod'
import {
  Alert02Icon,
  CheckmarkCircle02Icon,
  Loading03Icon,
  Refresh01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { Textarea } from '@/components/ui/textarea'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { getApiErrorTranslationKey } from '@/lib/api'
import { isIdString, parseIdString } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'
import { applyApiFieldErrors } from '@/lib/form-errors'
import { translateMessageRef } from '@/lib/message-ref'
import { useAuthStore } from '@/stores/auth-store'

import {
  backfillSite,
  createSite,
  getCollectionRun,
  preflightSiteBaseUrl,
  recheckSiteCapabilities,
} from '../api'
import { evaluateRequiredCapabilities } from '../capability-readiness'
import { siteKeys } from '../query-keys'
import {
  canRetrySiteUsageRun,
  onboardingBackfillRunContractError,
} from '../run-contract'
import {
  siteFormSchema,
  type SiteFormOutput,
  type SiteFormValues,
} from '../schema'
import type {
  SiteAuthorizationResult,
  SiteBaseUrlPreflightResult,
  SiteDetail,
} from '../types'
import { SiteAuthorizationForm } from './site-authorization-form'

const steps = ['basics', 'authorization', 'proof', 'backfill'] as const

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

export function SiteOnboardingDrawer({
  onComplete,
  onOpenChange,
  open,
}: {
  onComplete: (site: SiteDetail, runId?: string) => void
  onOpenChange: (open: boolean) => void
  open: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')
  const [step, setStep] = useState(0)
  const [submitting, setSubmitting] = useState(false)
  const [site, setSite] = useState<SiteDetail | null>(null)
  const [preflight, setPreflight] = useState<SiteBaseUrlPreflightResult | null>(
    null
  )
  const [authorization, setAuthorization] =
    useState<SiteAuthorizationResult | null>(null)
  const [historyConfirmed, setHistoryConfirmed] = useState(false)
  const [backfillRunId, setBackfillRunId] = useState<string | null>(null)
  const [backfillErrorKey, setBackfillErrorKey] = useState<string | null>(null)
  const [retryingBackfill, setRetryingBackfill] = useState(false)
  const [recheckingCapabilities, setRecheckingCapabilities] = useState(false)
  const {
    formState: { errors },
    handleSubmit,
    register,
    reset,
    setError,
  } = useForm<SiteFormValues, unknown, SiteFormOutput>({
    defaultValues: { baseUrl: '', name: '', remark: '' },
    resolver: zodResolver(siteFormSchema),
  })
  const validBackfillRunId = backfillRunId == null || isIdString(backfillRunId)
  const backfillRunQuery = useQuery({
    enabled: open && step === 3 && backfillRunId != null && validBackfillRunId,
    queryFn: () => getCollectionRun(parseIdString(backfillRunId ?? '')),
    queryKey: siteKeys.run(backfillRunId ?? ''),
    refetchInterval: (query) => {
      const run = query.state.data
      return run?.status === 'pending' || run?.status === 'running'
        ? 5_000
        : false
    },
    staleTime: 5_000,
  })
  const backfillRun = backfillRunQuery.data
  const capabilityReadiness = authorization
    ? evaluateRequiredCapabilities(authorization.capabilities)
    : null
  const backfillContractError =
    backfillRun && site
      ? onboardingBackfillRunContractError(backfillRun, site.id)
      : null

  useEffect(() => {
    if (!open) return
    setStep(0)
    setSite(null)
    setPreflight(null)
    setAuthorization(null)
    setHistoryConfirmed(false)
    setBackfillRunId(null)
    setBackfillErrorKey(null)
    setRecheckingCapabilities(false)
    reset({ baseUrl: '', name: '', remark: '' })
  }, [open, reset])

  const submitBasics = handleSubmit(async (values) => {
    setSubmitting(true)
    try {
      const created =
        site ??
        (await createSite({
          base_url: values.baseUrl,
          name: values.name,
          remark: values.remark || undefined,
        }))
      setSite(created)
      const result = await preflightSiteBaseUrl(created.id, values.baseUrl)
      setPreflight(result)
      setStep(1)
    } catch (error) {
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

  const close = () => onOpenChange(false)

  const retryBackfill = async () => {
    if (
      !site ||
      !backfillRun ||
      !canRetrySiteUsageRun(backfillRun, site.id) ||
      backfillRun.start_timestamp == null ||
      backfillRun.end_timestamp == null
    ) {
      return
    }
    setRetryingBackfill(true)
    setBackfillErrorKey(null)
    try {
      const nextRun = await backfillSite(site.id, {
        end_timestamp: backfillRun.end_timestamp,
        only_missing: true,
        start_timestamp: backfillRun.start_timestamp,
      })
      const contractError = onboardingBackfillRunContractError(nextRun, site.id)
      if (contractError) {
        setBackfillErrorKey(contractError)
        return
      }
      queryClient.setQueryData(siteKeys.run(nextRun.id), nextRun)
      setBackfillRunId(nextRun.id)
      toast.success(t('collection.retryQueued'))
    } catch (error) {
      setBackfillErrorKey(getApiErrorTranslationKey(error))
    } finally {
      setRetryingBackfill(false)
    }
  }

  const runCapabilityRecheck = async () => {
    if (!site || !isAdmin) return
    setRecheckingCapabilities(true)
    setBackfillErrorKey(null)
    try {
      const next = await recheckSiteCapabilities(site.id)
      setAuthorization(next)
      setBackfillRunId(next.backfill_run_id)
      void queryClient.invalidateQueries({ queryKey: siteKeys.all })
      toast.success(t('site.toast.capabilitiesRechecked'))
    } catch (error) {
      setBackfillErrorKey(getApiErrorTranslationKey(error))
    } finally {
      setRecheckingCapabilities(false)
    }
  }

  let backfillStepContent: ReactNode
  if (backfillRunId == null) {
    if (capabilityReadiness?.state === 'ready') {
      backfillStepContent = (
        <section className='border-success/30 bg-success/5 rounded-md border p-4 text-sm'>
          {t('site.onboarding.backfillNotRequired')}
        </section>
      )
    } else if (capabilityReadiness) {
      const pendingConfiguration =
        capabilityReadiness.state === 'pending_config'
      backfillStepContent = (
        <section
          className={
            pendingConfiguration
              ? 'border-warning/40 bg-warning/10 grid gap-3 rounded-md border p-4'
              : 'border-destructive/30 bg-destructive/5 grid gap-3 rounded-md border p-4'
          }
        >
          <div>
            <h3 className='font-medium'>
              {t(
                dynamicI18nKey(
                  'site',
                  pendingConfiguration
                    ? 'site.onboarding.capabilityPendingTitle'
                    : 'site.onboarding.capabilityErrorTitle'
                )
              )}
            </h3>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t(
                dynamicI18nKey(
                  'site',
                  pendingConfiguration
                    ? 'site.onboarding.capabilityPendingDescription'
                    : 'site.onboarding.capabilityErrorDescription'
                )
              )}
            </p>
          </div>
          {!capabilityReadiness.contractValid && (
            <p className='text-destructive text-sm' role='alert'>
              {t('site.onboarding.capabilityContractInvalid')}
            </p>
          )}
          <ul className='grid gap-2'>
            {capabilityReadiness.issues.map((issue) => (
              <li
                className='border-border bg-background rounded-md border p-3 text-sm'
                key={issue.key}
              >
                <div className='flex flex-wrap items-center justify-between gap-2'>
                  <strong>
                    {t(dynamicI18nKey('site', `site.capability.${issue.key}`))}
                  </strong>
                  <Badge
                    variant={
                      issue.status === 'failed' ? 'destructive' : 'neutral'
                    }
                  >
                    {t(
                      dynamicI18nKey(
                        'site',
                        `site.capability.status.${issue.status}`
                      )
                    )}
                  </Badge>
                </div>
                {issue.result?.message && (
                  <p className='text-muted-foreground mt-1'>
                    {translateMessageRef(issue.result.message)}
                  </p>
                )}
              </li>
            ))}
          </ul>
          {isAdmin && (
            <Button
              disabled={recheckingCapabilities}
              onClick={() => void runCapabilityRecheck()}
              variant='outline'
            >
              {recheckingCapabilities ? (
                <Spinner />
              ) : (
                <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
              )}
              {t('site.onboarding.recheckCapabilities')}
            </Button>
          )}
        </section>
      )
    } else {
      backfillStepContent = null
    }
  } else if (!validBackfillRunId) {
    backfillStepContent = (
      <p className='text-destructive text-sm' role='alert'>
        {t('collection.contract.foreignRun')}
      </p>
    )
  } else if (backfillRunQuery.isPending) {
    backfillStepContent = (
      <div
        aria-label={t('site.onboarding.backfillLoading')}
        className='flex min-h-40 items-center justify-center'
        role='status'
      >
        <Spinner />
      </div>
    )
  } else if (backfillRunQuery.isError && !backfillRun) {
    backfillStepContent = (
      <section className='border-destructive/30 bg-destructive/5 grid gap-3 rounded-md border p-4'>
        <p className='text-destructive text-sm' role='alert'>
          {t('site.onboarding.backfillLoadError')}
        </p>
        <Button
          onClick={() => void backfillRunQuery.refetch()}
          variant='outline'
        >
          <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
          {t('common.retry')}
        </Button>
      </section>
    )
  } else if (backfillContractError) {
    backfillStepContent = (
      <p className='text-destructive text-sm' role='alert'>
        {t(dynamicI18nKey('site', backfillContractError))}
      </p>
    )
  } else if (backfillRun) {
    let statusVariant: 'destructive' | 'success' | 'warning' = 'warning'
    if (backfillRun.status === 'success') statusVariant = 'success'
    else if (backfillRun.status === 'failed') statusVariant = 'destructive'

    let range = t('data.unavailableValue')
    if (
      backfillRun.start_timestamp != null &&
      backfillRun.end_timestamp != null
    ) {
      range = `${fromUnixSeconds(backfillRun.start_timestamp).format(
        'YYYY-MM-DD HH:00'
      )} - ${fromUnixSeconds(backfillRun.end_timestamp).format(
        'YYYY-MM-DD HH:00'
      )}`
    }
    const canRetry = site != null && canRetrySiteUsageRun(backfillRun, site.id)

    backfillStepContent = (
      <section className='border-border bg-muted/30 rounded-md border p-4'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div>
            <p className='text-muted-foreground text-xs'>
              {t('site.onboarding.backfillRunId')}
            </p>
            <p className='font-medium'>{backfillRun.id}</p>
          </div>
          <Badge variant={statusVariant}>
            {t(
              dynamicI18nKey('site', `collection.status.${backfillRun.status}`)
            )}
          </Badge>
        </div>
        <dl className='mt-4 grid gap-3 text-sm sm:grid-cols-2'>
          <div>
            <dt className='text-muted-foreground'>{t('collection.range')}</dt>
            <dd>{range}</dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>
              {t('collection.progress')}
            </dt>
            <dd>{Math.round(backfillRun.progress * 100)}%</dd>
          </div>
        </dl>
        {backfillRun.error && (
          <div className='mt-4'>
            <p className='text-destructive text-sm'>
              {translateMessageRef(backfillRun.error)}
            </p>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('collection.requestId', {
                requestId: backfillRun.last_request_id,
              })}
            </p>
          </div>
        )}
        {backfillRunQuery.isRefetchError && (
          <p className='text-warning-foreground mt-4 text-sm' role='status'>
            {t('site.onboarding.backfillRefreshError')}
          </p>
        )}
        {canRetry && (
          <Button
            className='mt-4'
            disabled={retryingBackfill}
            onClick={() => void retryBackfill()}
            variant='outline'
          >
            {retryingBackfill ? (
              <Spinner />
            ) : (
              <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
            )}
            {t('collection.retryFailedWindows')}
          </Button>
        )}
      </section>
    )
  } else {
    backfillStepContent = null
  }
  const requiresCapabilityRepair =
    backfillRunId == null &&
    capabilityReadiness != null &&
    capabilityReadiness.state !== 'ready'

  return (
    <Drawer direction='right' onOpenChange={onOpenChange} open={open}>
      <DrawerContent className='data-[vaul-drawer-direction=right]:sm:max-w-3xl'>
        <DrawerHeader>
          <DrawerTitle>{t('site.onboarding.title')}</DrawerTitle>
          <DrawerDescription>
            {t('site.onboarding.description')}
          </DrawerDescription>
        </DrawerHeader>

        <ol
          aria-label={t('site.onboarding.progress')}
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
                    ? 'bg-primary text-primary-foreground mx-auto flex size-8 items-center justify-center rounded-full text-sm font-medium'
                    : 'bg-muted text-muted-foreground mx-auto flex size-8 items-center justify-center rounded-full text-sm'
                }
              >
                {index + 1}
              </span>
              <span className='mt-1 block truncate text-xs'>
                {t(dynamicI18nKey('site', `site.onboarding.${item}`))}
              </span>
            </li>
          ))}
        </ol>

        {step === 0 && (
          <form
            className='grid gap-4'
            id='site-basics-form'
            noValidate
            onSubmit={submitBasics}
          >
            <FormField
              error={
                errors.name?.message &&
                t(dynamicI18nKey('site', errors.name.message))
              }
              htmlFor='onboarding-site-name'
              label={t('site.name')}
              required
            >
              <Input
                aria-invalid={Boolean(errors.name)}
                autoFocus
                disabled={site != null}
                id='onboarding-site-name'
                {...register('name')}
              />
            </FormField>
            <FormField
              description={t('site.baseUrlNormalization')}
              error={
                errors.baseUrl?.message &&
                t(dynamicI18nKey('site', errors.baseUrl.message))
              }
              htmlFor='onboarding-site-url'
              label={t('site.baseUrl')}
              required
            >
              <Input
                aria-invalid={Boolean(errors.baseUrl)}
                autoCapitalize='none'
                disabled={site != null}
                id='onboarding-site-url'
                inputMode='url'
                placeholder={t('site.baseUrlPlaceholder')}
                {...register('baseUrl')}
              />
            </FormField>
            <FormField
              error={
                errors.remark?.message &&
                t(dynamicI18nKey('site', errors.remark.message))
              }
              htmlFor='onboarding-site-remark'
              label={t('site.remark')}
            >
              <Textarea
                className='min-h-24'
                disabled={site != null}
                id='onboarding-site-remark'
                {...register('remark')}
              />
            </FormField>
            {site && !preflight && (
              <div className='border-warning/40 bg-warning/10 rounded-md border p-3 text-sm'>
                {t('site.onboarding.createdPreflightPending')}
              </div>
            )}
            {errors.root?.message && (
              <p className='text-destructive text-sm' role='alert'>
                {t(dynamicI18nKey('site', errors.root.message))}
              </p>
            )}
          </form>
        )}

        {step === 1 && site && (
          <div className='grid gap-4'>
            {preflight && (
              <section className='border-border bg-muted/30 rounded-lg border p-4'>
                <div className='flex items-center gap-2 font-medium'>
                  <HugeiconsIcon
                    className={
                      preflight.contract_status === 'compatible'
                        ? 'text-success'
                        : 'text-warning-foreground'
                    }
                    icon={
                      preflight.contract_status === 'compatible'
                        ? CheckmarkCircle02Icon
                        : Alert02Icon
                    }
                    strokeWidth={2}
                  />
                  {t(
                    dynamicI18nKey(
                      'site',
                      `site.preflight.${preflight.contract_status}`
                    )
                  )}
                </div>
                <dl className='mt-3 grid gap-2 text-sm sm:grid-cols-2'>
                  <div>
                    <dt className='text-muted-foreground'>
                      {t('site.normalizedUrl')}
                    </dt>
                    <dd className='break-all'>
                      {preflight.normalized_base_url}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground'>
                      {t('site.systemName')}
                    </dt>
                    <dd>{preflight.candidate_public.system_name}</dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground'>
                      {t('site.version')}
                    </dt>
                    <dd>{preflight.candidate_public.version}</dd>
                  </div>
                </dl>
              </section>
            )}
            <SiteAuthorizationForm
              formId='site-onboarding-authorization'
              onSuccess={(result) => {
                setAuthorization(result)
                setBackfillRunId(result.backfill_run_id)
                setStep(2)
              }}
              siteId={site.id}
              submitLabel={t('site.authorization.verify')}
            />
          </div>
        )}

        {step === 2 && authorization && (
          <div className='grid gap-4'>
            <section className='border-success/30 bg-success/5 rounded-lg border p-4'>
              <h2 className='flex items-center gap-2 font-medium'>
                <HugeiconsIcon icon={CheckmarkCircle02Icon} strokeWidth={2} />
                {t('site.proof.passed')}
              </h2>
              <dl className='mt-3 grid gap-3 text-sm sm:grid-cols-3'>
                <div>
                  <dt className='text-muted-foreground'>
                    {t('site.proof.snapshotTotal')}
                  </dt>
                  <dd>{authorization.first_user_proof.snapshot_total}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground'>
                    {t('site.proof.minUserId')}
                  </dt>
                  <dd>{authorization.first_user_proof.min_user_id}</dd>
                </div>
                <div>
                  <dt className='text-muted-foreground'>
                    {t('site.proof.earliestCreated')}
                  </dt>
                  <dd>
                    {fromUnixSeconds(
                      authorization.first_user_proof.earliest_created_at
                    ).format('YYYY-MM-DD HH:mm:ss')}
                  </dd>
                </div>
              </dl>
            </section>
            <section>
              <h2 className='font-medium'>{t('site.capabilities.title')}</h2>
              <ul className='mt-2 grid gap-2'>
                {authorization.capabilities.map((capability) => {
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
                        <strong className='block font-medium'>
                          {t(
                            dynamicI18nKey(
                              'site',
                              `site.capability.${capability.key}`
                            ),
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
            </section>
            <section className='border-primary/30 bg-primary/5 rounded-lg border p-4'>
              <h2 className='font-medium'>
                {t('site.history.immutableTitle')}
              </h2>
              <dl className='mt-3 grid gap-3 text-sm sm:grid-cols-2'>
                <div>
                  <dt className='text-muted-foreground'>
                    {t('site.rootCreatedAt')}
                  </dt>
                  <dd>
                    {fromUnixSeconds(authorization.root_created_at).format(
                      'YYYY-MM-DD HH:mm:ss'
                    )}
                  </dd>
                </div>
                <div>
                  <dt className='text-muted-foreground'>
                    {t('site.statisticsStartAt')}
                  </dt>
                  <dd>
                    {fromUnixSeconds(authorization.statistics_start_at).format(
                      'YYYY-MM-DD HH:00'
                    )}
                  </dd>
                </div>
              </dl>
              <p className='text-muted-foreground mt-3 text-sm'>
                {t('site.history.immutableDescription')}
              </p>
            </section>
            <label className='border-border flex min-h-12 items-start gap-3 rounded-md border p-3 text-sm'>
              <Checkbox
                checked={historyConfirmed}
                className='mt-0.5'
                onCheckedChange={setHistoryConfirmed}
              />
              {t('site.history.confirm')}
            </label>
          </div>
        )}

        {step === 3 && authorization && site && (
          <div className='grid gap-4'>
            <div>
              <h2 className='font-medium'>
                {t('site.onboarding.backfillTitle')}
              </h2>
              <p className='text-muted-foreground mt-1 text-sm'>
                {t('site.onboarding.backfillDescription')}
              </p>
            </div>
            {backfillStepContent}
            {backfillErrorKey && (
              <p className='text-destructive text-sm' role='alert'>
                {t(dynamicI18nKey('site', backfillErrorKey))}
              </p>
            )}
          </div>
        )}

        <DrawerFooter>
          <Button onClick={close} type='button' variant='outline'>
            {t('common.cancel')}
          </Button>
          {step === 0 && (
            <Button disabled={submitting} form='site-basics-form' type='submit'>
              {submitting && <Spinner />}
              {site
                ? t('site.preflight.retry')
                : t('site.onboarding.createAndPreflight')}
            </Button>
          )}
          {step === 2 && (
            <Button
              disabled={!historyConfirmed}
              onClick={() => setStep(3)}
              type='button'
            >
              {t('common.continue')}
            </Button>
          )}
          {step === 3 && site && (
            <Button
              disabled={
                recheckingCapabilities ||
                (backfillRunId != null &&
                  (!validBackfillRunId ||
                    !backfillRun ||
                    Boolean(backfillContractError)))
              }
              onClick={() => {
                onComplete(site, backfillRun?.id)
                close()
              }}
              type='button'
            >
              {t(
                dynamicI18nKey(
                  'site',
                  requiresCapabilityRepair
                    ? 'site.onboarding.enterDetailToRepair'
                    : 'site.onboarding.finish'
                )
              )}
            </Button>
          )}
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  )
}
