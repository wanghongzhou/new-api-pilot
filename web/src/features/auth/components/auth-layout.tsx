import type { ReactNode } from 'react'

import { Brand } from '@/components/layout/brand'

export function AuthLayout({
  children,
  standalone = false,
}: {
  children: ReactNode
  standalone?: boolean
}) {
  return (
    <main
      className={
        standalone
          ? 'relative grid h-svh max-w-none'
          : 'relative grid min-h-full max-w-none'
      }
      id='main-content'
      tabIndex={-1}
    >
      {standalone && (
        <div className='absolute top-4 left-4 z-10 transition-opacity hover:opacity-80 sm:top-8 sm:left-8'>
          <Brand />
        </div>
      )}
      <div className='container flex items-center pt-16 sm:pt-0'>
        <div className='mx-auto flex w-full flex-col justify-center space-y-2 px-4 py-8 sm:w-[480px] sm:p-8'>
          {children}
        </div>
      </div>
    </main>
  )
}
