import { AlertDialog as BaseAlertDialog } from '@base-ui/react/alert-dialog'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from './button'
import { Spinner } from './spinner'

interface ConfirmDialogProps {
  confirmLabel: string
  description: ReactNode
  onConfirm: () => void
  onOpenChange: (open: boolean) => void
  open: boolean
  pending?: boolean
  title: string
  variant?: 'primary' | 'destructive'
}

export function ConfirmDialog({
  confirmLabel,
  description,
  onConfirm,
  onOpenChange,
  open,
  pending = false,
  title,
  variant = 'destructive',
}: ConfirmDialogProps) {
  const { t } = useTranslation()
  return (
    <BaseAlertDialog.Root onOpenChange={onOpenChange} open={open}>
      <BaseAlertDialog.Portal>
        <BaseAlertDialog.Backdrop className='data-open:animate-in data-closed:animate-out data-open:fade-in data-closed:fade-out fixed inset-0 z-50 bg-black/35 backdrop-blur-[1px]' />
        <BaseAlertDialog.Popup className='bg-popover text-popover-foreground data-open:animate-in data-closed:animate-out data-open:fade-in data-open:zoom-in-95 data-closed:fade-out data-closed:zoom-out-95 fixed top-1/2 left-1/2 z-50 grid max-h-[calc(100svh-2rem)] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 gap-4 overflow-y-auto rounded-lg border p-5 shadow-xl'>
          <div className='grid gap-1.5'>
            <BaseAlertDialog.Title className='text-base font-semibold'>
              {title}
            </BaseAlertDialog.Title>
            <BaseAlertDialog.Description className='text-muted-foreground text-sm'>
              {description}
            </BaseAlertDialog.Description>
          </div>
          <div className='flex flex-col-reverse gap-2 sm:flex-row sm:justify-end'>
            <BaseAlertDialog.Close
              render={<Button autoFocus disabled={pending} variant='outline' />}
            >
              {t('common.cancel')}
            </BaseAlertDialog.Close>
            <Button disabled={pending} onClick={onConfirm} variant={variant}>
              {pending && <Spinner />}
              {confirmLabel}
            </Button>
          </div>
        </BaseAlertDialog.Popup>
      </BaseAlertDialog.Portal>
    </BaseAlertDialog.Root>
  )
}
