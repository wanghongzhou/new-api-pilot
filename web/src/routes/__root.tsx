import type { QueryClient } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { Outlet, createRootRouteWithContext } from '@tanstack/react-router'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'
import { Toaster } from 'sonner'

import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider, useTheme } from '@/context/theme-provider'
import { useIsMobile } from '@/hooks/use-mobile'

function AppToaster() {
  const { resolvedTheme } = useTheme()
  const isMobile = useIsMobile()
  return (
    <Toaster
      closeButton
      position={isMobile ? 'bottom-center' : 'top-center'}
      theme={resolvedTheme}
      toastOptions={{
        classNames: {
          closeButton: 'pointer-events-auto',
          toast: 'pointer-events-none',
        },
      }}
    />
  )
}

function RootComponent() {
  return (
    <ThemeProvider>
      <ThemeCustomizationProvider>
        <Outlet />
        <AppToaster />
        {import.meta.env.MODE === 'development' && (
          <>
            <ReactQueryDevtools buttonPosition='bottom-left' />
            <TanStackRouterDevtools position='bottom-right' />
          </>
        )}
      </ThemeCustomizationProvider>
    </ThemeProvider>
  )
}

export const Route = createRootRouteWithContext<{
  queryClient: QueryClient
}>()({
  component: RootComponent,
})
