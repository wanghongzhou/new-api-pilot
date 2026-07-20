import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

export interface SectionPageLayoutProps {
  actions?: ReactNode
  children: ReactNode
  description?: ReactNode
  fixedContent?: boolean
  title: ReactNode
}

export function SectionPageLayout({
  actions,
  children,
  description,
  fixedContent = false,
  title,
}: SectionPageLayoutProps) {
  return (
    <main
      className={cn(
        'flex h-full min-h-0 max-h-full flex-1 flex-col',
        fixedContent
          ? 'overflow-hidden'
          : 'overflow-y-auto overscroll-y-contain'
      )}
      id='main-content'
      tabIndex={-1}
    >
      <header className='border-border bg-background sticky top-0 z-10 flex flex-wrap items-start justify-between gap-3 border-b px-4 py-4 sm:px-6'>
        <div className='min-w-0'>
          <h1 className='text-xl font-semibold'>{title}</h1>
          {description && (
            <p className='text-muted-foreground mt-1 text-sm'>{description}</p>
          )}
        </div>
        {actions && <div className='flex flex-wrap gap-2'>{actions}</div>}
      </header>
      <div
        className={cn(
          'w-full flex-1 px-4 py-4 sm:px-6 sm:py-6',
          fixedContent && 'min-h-0 overflow-hidden'
        )}
      >
        {children}
      </div>
    </main>
  )
}
