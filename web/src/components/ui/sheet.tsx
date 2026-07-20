import { Dialog as BaseDialog } from '@base-ui/react/dialog'
import { Cancel01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ComponentProps, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

export const Sheet = BaseDialog.Root
export const SheetClose = BaseDialog.Close

export function SheetContent({
  children,
  className,
  ...props
}: BaseDialog.Popup.Props & { children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <BaseDialog.Portal>
      <BaseDialog.Backdrop className='data-open:animate-in data-closed:animate-out data-open:fade-in data-closed:fade-out fixed inset-0 z-50 bg-black/35 backdrop-blur-[1px]' />
      <BaseDialog.Popup
        className={cn(
          'bg-popover text-popover-foreground data-open:animate-in data-closed:animate-out data-open:slide-in-from-right data-closed:slide-out-to-right fixed inset-y-0 right-0 z-50 grid h-svh w-full max-w-2xl content-start gap-4 overflow-y-auto border-l p-5 shadow-xl outline-none sm:w-[min(42rem,calc(100vw-3rem))]',
          className
        )}
        {...props}
      >
        {children}
        <BaseDialog.Close
          aria-label={t('common.close')}
          className='hover:bg-muted focus-visible:ring-ring absolute top-2 right-2 flex size-10 items-center justify-center rounded-md outline-none focus-visible:ring-2'
          title={t('common.close')}
        >
          <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
        </BaseDialog.Close>
      </BaseDialog.Popup>
    </BaseDialog.Portal>
  )
}

export function SheetHeader({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('grid gap-1.5 pr-10', className)} {...props} />
}

export function SheetTitle({ className, ...props }: BaseDialog.Title.Props) {
  return (
    <BaseDialog.Title
      className={cn('text-lg font-semibold', className)}
      {...props}
    />
  )
}

export function SheetDescription({
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
