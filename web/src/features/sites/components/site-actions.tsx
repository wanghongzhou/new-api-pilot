import {
  Activity03Icon,
  Archive03Icon,
  Delete02Icon,
  Edit03Icon,
  Key01Icon,
  MoreVerticalIcon,
  Refresh01Icon,
  RotateClockwiseIcon,
  StopCircleIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import type { SiteListItem } from '../types'

export type SiteAction =
  | 'edit'
  | 'authorize'
  | 'recheck'
  | 'probe'
  | 'refresh'
  | 'backfill'
  | 'disable'
  | 'manage-lifecycle'
  | 'enable'
  | 'end-statistics'
  | 'clear-statistics-end'
  | 'delete'

const actionIcons = {
  authorize: Key01Icon,
  backfill: RotateClockwiseIcon,
  'clear-statistics-end': Refresh01Icon,
  delete: Delete02Icon,
  disable: Archive03Icon,
  edit: Edit03Icon,
  enable: Activity03Icon,
  'end-statistics': StopCircleIcon,
  'manage-lifecycle': Archive03Icon,
  probe: Activity03Icon,
  recheck: Refresh01Icon,
  refresh: Refresh01Icon,
} as const

function actionsForSite(site: SiteListItem): SiteAction[] {
  return [
    'edit',
    'authorize',
    'recheck',
    'probe',
    'refresh',
    'backfill',
    site.management_status === 'active' ? 'disable' : 'manage-lifecycle',
    'delete',
  ]
}

export function SiteActions({
  onAction,
  site,
}: {
  onAction: (action: SiteAction, site: SiteListItem) => void
  site: SiteListItem
}) {
  const { t } = useTranslation()
  return (
    <details className='relative'>
      <summary
        aria-label={t('site.actions.open')}
        className='hover:bg-muted focus-visible:ring-ring flex size-10 list-none items-center justify-center rounded-md outline-none focus-visible:ring-2 [&::-webkit-details-marker]:hidden'
        title={t('site.actions.open')}
      >
        <HugeiconsIcon icon={MoreVerticalIcon} strokeWidth={2} />
      </summary>
      <div className='bg-popover text-popover-foreground absolute top-11 right-0 z-30 grid min-w-52 rounded-md border p-1 shadow-lg'>
        {actionsForSite(site).map((action) => {
          const Icon = actionIcons[action]
          return (
            <button
              className={
                action === 'delete'
                  ? 'text-destructive hover:bg-destructive/10 flex min-h-10 items-center gap-2 rounded-sm px-3 text-left text-sm'
                  : 'hover:bg-muted flex min-h-10 items-center gap-2 rounded-sm px-3 text-left text-sm'
              }
              key={action}
              onClick={(event) => {
                event.currentTarget.closest('details')?.removeAttribute('open')
                onAction(action, site)
              }}
              type='button'
            >
              <HugeiconsIcon icon={Icon} strokeWidth={2} />
              {t(dynamicI18nKey('site', `site.actions.${action}`))}
            </button>
          )
        })}
      </div>
    </details>
  )
}
