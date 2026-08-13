import type { QueryClient } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { Outlet, createRootRouteWithContext } from '@tanstack/react-router'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'
import { useState } from 'react'

import {
  RouteErrorState,
  RouteNotFoundState,
} from '@/components/layout/route-error-state'
import { Toaster } from '@/components/ui/sonner'
import { DirectionProvider } from '@/context/direction-provider'
import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'

function DevelopmentTools() {
  useState(() => {
    // Router devtools persists its open state. A previous debugging session
    // must not cover the application after a refresh, especially on mobile.
    localStorage.removeItem('tanstackRouterDevtoolsOpen')
  })

  return (
    <>
      <ReactQueryDevtools buttonPosition='bottom-left' initialIsOpen={false} />
      <TanStackRouterDevtools initialIsOpen={false} position='bottom-right' />
    </>
  )
}

function RootComponent() {
  return (
    <ThemeProvider>
      <DirectionProvider>
        <ThemeCustomizationProvider>
          <Outlet />
          <Toaster
            closeButton
            duration={5000}
            position='top-center'
            richColors
          />
          {import.meta.env.MODE === 'development' && <DevelopmentTools />}
        </ThemeCustomizationProvider>
      </DirectionProvider>
    </ThemeProvider>
  )
}

export const Route = createRootRouteWithContext<{
  queryClient: QueryClient
}>()({
  component: RootComponent,
  errorComponent: RouteErrorState,
  notFoundComponent: RouteNotFoundState,
})
