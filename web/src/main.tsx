import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import { StrictMode } from 'react'
import ReactDOM from 'react-dom/client'
import { toast } from 'sonner'

import { clearAuthenticatedSession } from './features/auth/session'
import i18n from './i18n/config'
import { setUnauthorizedHandler } from './lib/api'
import { queryClient } from './lib/query-client'
import { parseRouterSearch, stringifyRouterSearch } from './lib/router-search'
import { routeTree } from './routeTree.gen'
import { useAuthStore } from './stores/auth-store'

import './styles/index.css'

document.title = i18n.t('app.name')
document
  .querySelector('meta[name="description"]')
  ?.setAttribute('content', i18n.t('app.description'))

const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 0,
  parseSearch: parseRouterSearch,
  stringifySearch: stringifyRouterSearch,
})

if (import.meta.env.DEV) {
  Object.defineProperty(window, '__PILOT_CACHE_SNAPSHOT__', {
    configurable: true,
    value: () => ({
      mutations: queryClient
        .getMutationCache()
        .getAll()
        .map((mutation) => ({
          data: mutation.state.data,
          error:
            mutation.state.error instanceof Error
              ? mutation.state.error.message
              : mutation.state.error,
          variables: mutation.state.variables,
        })),
      queries: queryClient
        .getQueryCache()
        .getAll()
        .map((query) => ({
          data: query.state.data,
          error:
            query.state.error instanceof Error
              ? query.state.error.message
              : query.state.error,
          queryKey: query.queryKey,
        })),
    }),
  })
}

setUnauthorizedHandler(() => {
  if (!useAuthStore.getState().user) return
  const redirect = router.state.location.href
  clearAuthenticatedSession()
  toast.error(i18n.t('Session expired'))
  void router.navigate({
    replace: true,
    search: { redirect },
    to: '/sign-in',
  })
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

const rootElement = document.querySelector('#root')
if (!rootElement) {
  throw new Error('Root element was not found')
}

ReactDOM.createRoot(rootElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>
)
