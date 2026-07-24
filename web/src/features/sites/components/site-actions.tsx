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

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
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
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            aria-label={t('site.actions.open')}
            size='icon'
            title={t('site.actions.open')}
            variant='ghost'
          />
        }
      >
        <HugeiconsIcon icon={MoreVerticalIcon} strokeWidth={2} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='min-w-52'>
        {actionsForSite(site).map((action) => {
          const Icon = actionIcons[action]
          return (
            <DropdownMenuItem
              key={action}
              onClick={() => onAction(action, site)}
              variant={action === 'delete' ? 'destructive' : 'default'}
            >
              <HugeiconsIcon icon={Icon} strokeWidth={2} />
              {t(dynamicI18nKey('site', `site.actions.${action}`))}
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
