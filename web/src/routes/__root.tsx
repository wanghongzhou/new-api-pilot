import type { QueryClient } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { Outlet, createRootRouteWithContext } from '@tanstack/react-router'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'
import { Toaster } from 'sonner'

import { ThemeProvider, useTheme } from '@/context/theme-provider'

function AppToaster() {
  const { resolvedTheme } = useTheme()
  return <Toaster closeButton position='top-center' theme={resolvedTheme} />
}

function RootComponent() {
  return (
    <ThemeProvider>
      <Outlet />
      <AppToaster />
      {import.meta.env.MODE === 'development' && (
        <>
          <ReactQueryDevtools buttonPosition='bottom-left' />
          <TanStackRouterDevtools position='bottom-right' />
        </>
      )}
    </ThemeProvider>
  )
}

export const Route = createRootRouteWithContext<{
  queryClient: QueryClient
}>()({
  component: RootComponent,
})
