import { Dialog as BaseDialog } from '@base-ui/react/dialog'
import { Cancel01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ComponentProps, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { Button } from './button'

export const Dialog = BaseDialog.Root
export const DialogTrigger = BaseDialog.Trigger
export const DialogClose = BaseDialog.Close

export function DialogContent({
  children,
  className,
  showClose = true,
  ...props
}: BaseDialog.Popup.Props & {
  children: ReactNode
  showClose?: boolean
}) {
  const { t } = useTranslation()

  return (
    <BaseDialog.Portal>
      <BaseDialog.Backdrop className='data-open:animate-in data-closed:animate-out data-open:fade-in data-closed:fade-out fixed inset-0 z-50 bg-black/35 backdrop-blur-[1px]' />
      <BaseDialog.Popup
        className={cn(
          'bg-popover text-popover-foreground data-open:animate-in data-closed:animate-out fixed top-1/2 left-1/2 z-50 grid max-h-[calc(100svh-2rem)] w-[calc(100%-2rem)] max-w-lg -translate-x-1/2 -translate-y-1/2 gap-4 overflow-y-auto rounded-lg border p-5 shadow-xl data-open:fade-in data-open:zoom-in-95 data-closed:fade-out data-closed:zoom-out-95',
          className
        )}
        {...props}
      >
        {children}
        {showClose && (
          <BaseDialog.Close
            aria-label={t('Close')}
            className='hover:bg-muted focus-visible:ring-ring absolute top-2 right-2 flex size-10 items-center justify-center rounded-md outline-none focus-visible:ring-2'
            title={t('Close')}
          >
            <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
          </BaseDialog.Close>
        )}
      </BaseDialog.Popup>
    </BaseDialog.Portal>
  )
}

export function DialogHeader({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('grid gap-1.5 pr-10', className)} {...props} />
}

export function DialogTitle({ className, ...props }: BaseDialog.Title.Props) {
  return (
    <BaseDialog.Title
      className={cn('text-base font-semibold', className)}
      {...props}
    />
  )
}

export function DialogDescription({
  className,
  ...props
}: BaseDialog.Description.Props) {
  return (
    <BaseDialog.Description
      className={cn('text-muted-foreground text-sm', className)}
      {...props}
    />
  )
}

export function DialogFooter({
  children,
  className,
  ...props
}: ComponentProps<'div'>) {
  return (
    <div
      className={cn(
        'mt-2 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end',
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
}

export function DialogCancelButton() {
  const { t } = useTranslation()
  return (
    <BaseDialog.Close render={<Button autoFocus variant='outline' />}>
      {t('Cancel')}
    </BaseDialog.Close>
  )
}
