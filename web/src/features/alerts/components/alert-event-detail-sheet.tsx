import { Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { normalizeApiError } from '@/lib/api'
import { isIdString } from '@/lib/api-types'
import { translateMessageRef } from '@/lib/message-ref'

import { getAlert } from '../api'
import { alertKeys } from '../query-keys'
import type { AlertDeliveryItem, AlertEventDetail } from '../types'
import {
  alertDeliveryErrorText,
  AlertLevelBadge,
  AlertStatusBadge,
  AlertTime,
  alertRuleName,
  alertTargetTypeText,
  DeliveryStatusBadge,
  deliveryEventTypeText,
} from './alert-ui'

function DetailLink({ detail }: { detail: AlertEventDetail }) {
  const { t } = useTranslation()
  if (detail.target_type === 'account' && isIdString(detail.target_key)) {
    return (
      <Link
        className='text-primary-strong text-sm font-medium hover:underline'
        params={{ accountId: detail.target_key }}
        to='/accounts/$accountId'
      >
        {t('alerts.detail.openAccount')}
      </Link>
    )
  }
  if (detail.site_id) {
    return (
      <Link
        className='text-primary-strong text-sm font-medium hover:underline'
        params={{ siteId: detail.site_id }}
        to='/sites/$siteId'
      >
        {t('alerts.detail.openSite')}
      </Link>
    )
  }
  return null
}

function DeliveryRow({ delivery }: { delivery: AlertDeliveryItem }) {
  const { t } = useTranslation()
  return (
    <article className='grid gap-3 py-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex flex-wrap items-center gap-2'>
          <DeliveryStatusBadge status={delivery.status} />
          <span className='text-sm font-medium'>
            {deliveryEventTypeText(t, delivery.event_type)}
          </span>
        </div>
        <span className='text-muted-foreground text-xs'>
          {t('alerts.delivery.attempts', { count: delivery.attempt_count })}
        </span>
      </div>
      <dl className='grid gap-3 text-sm sm:grid-cols-2'>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.delivery.result')}
          </dt>
          <dd>{alertDeliveryErrorText(t, delivery.error_code)}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.delivery.responseCode')}
          </dt>
          <dd>{delivery.response_code ?? t('alerts.value.unavailable')}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.delivery.nextRetryAt')}
          </dt>
          <dd>
            <AlertTime timestamp={delivery.next_retry_at} />
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('alerts.delivery.sentAt')}
          </dt>
          <dd>
            <AlertTime timestamp={delivery.sent_at} />
          </dd>
        </div>
      </dl>
      {delivery.response_message && (
        <details className='text-sm'>
          <summary className='min-h-10 cursor-pointer py-2 font-medium'>
            {t('alerts.delivery.technical')}
          </summary>
          <p className='bg-muted/45 rounded-md p-3 break-words whitespace-pre-wrap'>
            {delivery.response_message}
          </p>
        </details>
      )}
    </article>
  )
}

function DetailContent({ detail }: { detail: AlertEventDetail }) {
  const { t } = useTranslation()
  const technicalDetail = detail.message.technical_detail
  const resolutionReasonText = (() => {
    switch (detail.resolution_reason) {
      case 'recovered':
        return t('alerts.resolution.recovered')
      case 'remediated':
        return t('alerts.resolution.remediated')
      case 'retired':
        return t('alerts.resolution.retired')
      case 'superseded':
        return t('alerts.resolution.superseded')
      default:
        return null
    }
  })()
  const timelineItems = [
    {
      key: 'first-observed',
      label: t('alerts.detail.firstObservedAt'),
      timestamp: detail.first_observed_at,
    },
    {
      key: 'first-fired',
      label: t('alerts.table.firstFiredAt'),
      timestamp: detail.first_fired_at,
    },
    {
      key: 'last-fired',
      label: t('alerts.table.lastFiredAt'),
      timestamp: detail.last_fired_at,
    },
    {
      key: 'resolved',
      label: t('alerts.table.resolvedAt'),
      timestamp: detail.resolved_at,
    },
  ]
  return (
    <div className='grid gap-6'>
      <section aria-labelledby='alert-evidence-title' className='grid gap-3'>
        <div className='flex flex-wrap gap-2'>
          <AlertLevelBadge level={detail.level} />
          <AlertStatusBadge status={detail.status} />
        </div>
        <div>
          <h3 className='font-semibold' id='alert-evidence-title'>
            {t('alerts.detail.evidence')}
          </h3>
          <p className='mt-1 text-sm break-words'>
            {translateMessageRef(detail.message)}
          </p>
        </div>
        {resolutionReasonText && (
          <p className='text-sm'>
            <span className='font-medium'>
              {t('alerts.detail.resolutionReason')}:
            </span>{' '}
            {resolutionReasonText}
          </p>
        )}
        {technicalDetail && (
          <details className='text-sm'>
            <summary className='min-h-10 cursor-pointer py-2 font-medium'>
              {t('alerts.detail.technical')}
            </summary>
            <p className='bg-muted/45 rounded-md p-3 break-words whitespace-pre-wrap'>
              {technicalDetail}
            </p>
          </details>
        )}
      </section>

      <section aria-labelledby='alert-object-title'>
        <h3 className='font-semibold' id='alert-object-title'>
          {t('alerts.detail.object')}
        </h3>
        <dl className='border-border divide-border mt-2 grid divide-y border-y text-sm'>
          <div className='grid gap-1 py-3 sm:grid-cols-[10rem_1fr]'>
            <dt className='text-muted-foreground'>{t('alerts.table.rule')}</dt>
            <dd>{alertRuleName(t, detail.rule_key)}</dd>
          </div>
          <div className='grid gap-1 py-3 sm:grid-cols-[10rem_1fr]'>
            <dt className='text-muted-foreground'>{t('alerts.table.site')}</dt>
            <dd>{detail.site_name || t('alerts.value.unavailable')}</dd>
          </div>
          <div className='grid gap-1 py-3 sm:grid-cols-[10rem_1fr]'>
            <dt className='text-muted-foreground'>
              {alertTargetTypeText(t, detail.target_type)}
            </dt>
            <dd className='break-words'>{detail.target_name}</dd>
          </div>
          <div className='grid gap-1 py-3 sm:grid-cols-[10rem_1fr]'>
            <dt className='text-muted-foreground'>{t('alerts.table.value')}</dt>
            <dd>
              {t('alerts.value.currentThreshold', {
                current: detail.current_value ?? t('alerts.value.unavailable'),
                threshold:
                  detail.threshold_value ?? t('alerts.value.unavailable'),
              })}
            </dd>
          </div>
        </dl>
        <div className='mt-3'>
          <DetailLink detail={detail} />
        </div>
      </section>

      <section aria-labelledby='alert-timeline-title'>
        <h3 className='font-semibold' id='alert-timeline-title'>
          {t('alerts.detail.timeline')}
        </h3>
        <dl className='border-border divide-border mt-2 grid divide-y border-y text-sm'>
          {timelineItems.map((item) => (
            <div
              className='grid gap-1 py-3 sm:grid-cols-[10rem_1fr]'
              key={item.key}
            >
              <dt className='text-muted-foreground'>{item.label}</dt>
              <dd>
                <AlertTime timestamp={Number(item.timestamp) || null} />
              </dd>
            </div>
          ))}
          <div className='grid gap-1 py-3 sm:grid-cols-[10rem_1fr]'>
            <dt className='text-muted-foreground'>
              {t('alerts.detail.consecutiveCount')}
            </dt>
            <dd>{detail.consecutive_count}</dd>
          </div>
        </dl>
      </section>

      <section aria-labelledby='alert-deliveries-title'>
        <h3 className='font-semibold' id='alert-deliveries-title'>
          {t('alerts.detail.deliveries')}
        </h3>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t('alerts.detail.deliveriesDescription')}
        </p>
        {detail.deliveries.length === 0 ? (
          <p className='border-border mt-3 border-y py-5 text-sm'>
            {t('alerts.detail.noDeliveries')}
          </p>
        ) : (
          <div className='border-border divide-border mt-3 divide-y border-y'>
            {detail.deliveries.map((delivery) => (
              <DeliveryRow delivery={delivery} key={delivery.id} />
            ))}
          </div>
        )}
      </section>
    </div>
  )
}

export function AlertEventDetailSheet({
  alertId,
  onClose,
}: {
  alertId?: AlertEventDetail['id']
  onClose: () => void
}) {
  const { t } = useTranslation()
  const query = useQuery({
    enabled: Boolean(alertId),
    queryFn: () => {
      if (!alertId) throw new Error()
      return getAlert(alertId)
    },
    queryKey: alertId ? alertKeys.detail(alertId) : alertKeys.all,
    staleTime: 30_000,
  })
  const notFound =
    query.isError && normalizeApiError(query.error).status === 404
  return (
    <Sheet onOpenChange={(open) => !open && onClose()} open={Boolean(alertId)}>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t('alerts.detail.title')}</SheetTitle>
          <SheetDescription>{t('alerts.detail.description')}</SheetDescription>
        </SheetHeader>
        {query.isPending && (
          <div className='grid gap-3' role='status'>
            <Spinner />
            <span>{t('alerts.detail.loading')}</span>
          </div>
        )}
        {query.isError && (
          <section className='border-destructive/30 bg-destructive/5 rounded-md border p-4'>
            <h3 className='font-medium'>
              {notFound
                ? t('alerts.detail.notFound')
                : t('alerts.detail.loadError')}
            </h3>
            <p className='text-muted-foreground mt-1 text-sm'>
              {notFound
                ? t('alerts.detail.notFoundDescription')
                : t('alerts.detail.loadErrorDescription')}
            </p>
            {!notFound && (
              <Button
                className='mt-3'
                onClick={() => void query.refetch()}
                variant='outline'
              >
                <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
                {t('common.retry')}
              </Button>
            )}
          </section>
        )}
        {query.data && <DetailContent detail={query.data} />}
      </SheetContent>
    </Sheet>
  )
}
