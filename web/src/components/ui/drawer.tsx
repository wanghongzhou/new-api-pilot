import { Drawer as BaseDrawer } from '@base-ui/react/drawer'
import { Cancel01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ComponentProps, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

export const Drawer = BaseDrawer.Root
export const DrawerTrigger = BaseDrawer.Trigger
export const DrawerClose = BaseDrawer.Close

export function DrawerContent({
  children,
  className,
  ...props
}: BaseDrawer.Popup.Props & { children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <BaseDrawer.Portal>
      <BaseDrawer.Backdrop className='data-open:animate-in data-closed:animate-out data-open:fade-in data-closed:fade-out fixed inset-0 z-50 bg-black/35 backdrop-blur-[1px]' />
      <BaseDrawer.Viewport className='fixed inset-0 z-50 flex items-stretch justify-end'>
        <BaseDrawer.Popup
          className={cn(
            'bg-popover text-popover-foreground data-starting-style:translate-x-full data-ending-style:translate-x-full relative h-full w-full max-w-3xl overflow-y-auto border-l p-5 shadow-xl outline-none transition-transform duration-300 sm:w-[min(48rem,calc(100vw-3rem))]',
            className
          )}
          {...props}
        >
          {children}
          <BaseDrawer.Close
            aria-label={t('common.close')}
            className='hover:bg-muted focus-visible:ring-ring absolute top-2 right-2 flex size-10 items-center justify-center rounded-md outline-none focus-visible:ring-2'
            title={t('common.close')}
          >
            <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
          </BaseDrawer.Close>
        </BaseDrawer.Popup>
      </BaseDrawer.Viewport>
    </BaseDrawer.Portal>
  )
}

export function DrawerHeader({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('grid gap-1.5 pr-10', className)} {...props} />
}

export function DrawerTitle({ className, ...props }: BaseDrawer.Title.Props) {
  return (
    <BaseDrawer.Title
      className={cn('text-lg font-semibold', className)}
      {...props}
    />
  )
}

export function DrawerDescription({
  className,
  ...props
}: BaseDrawer.Description.Props) {
  return (
    <BaseDrawer.Description
      className={cn('text-muted-foreground text-sm', className)}
      {...props}
    />
  )
}

export function DrawerFooter({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      className={cn(
        'border-border bg-popover sticky bottom-0 mt-6 flex flex-col-reverse gap-2 border-t py-4 sm:flex-row sm:justify-end',
        className
      )}
      {...props}
    />
  )
}
