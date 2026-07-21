import { useMutation } from '@tanstack/react-query'
import { Outlet, useRouter } from '@tanstack/react-router'
import { useEffect } from 'react'

import { AnimatedOutlet } from '@/components/page-transition'
import { SkipToMain } from '@/components/skip-to-main'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { LayoutProvider } from '@/context/layout-provider'
import { logout } from '@/features/auth/api'
import { clearAuthenticatedSession } from '@/features/auth/session'
import { getCookie } from '@/lib/cookies'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { AppHeader } from './app-header'
import { AppSidebar } from './app-sidebar'

export function AuthenticatedLayout() {
  const router = useRouter()
  const user = useAuthStore((state) => state.user)
  const logoutMutation = useMutation({
    mutationFn: logout,
    onSettled: () => {
      clearAuthenticatedSession()
      void router.navigate({ to: '/sign-in' })
    },
  })

  useEffect(() => {
    const root = document.querySelector('#root')
    root?.classList.add('h-svh', 'overflow-hidden')
    return () => {
      root?.classList.remove('h-svh', 'overflow-hidden')
    }
  }, [])

  if (!user) return null

  const header = (
    <AppHeader
      isLoggingOut={logoutMutation.isPending}
      onLogout={() => logoutMutation.mutate()}
      showMenu={!user.must_change_password}
      user={user}
    />
  )

  if (user.must_change_password) {
    return (
      <LayoutProvider>
        <SidebarProvider className='flex-col'>
          {header}
          <div className='bg-background min-h-0 flex-1 overflow-y-auto'>
            <Outlet />
          </div>
        </SidebarProvider>
      </LayoutProvider>
    )
  }

  const defaultOpen = getCookie('sidebar_state') !== 'false'

  return (
    <LayoutProvider>
      <SidebarProvider className='flex-col' defaultOpen={defaultOpen}>
        <SkipToMain />
        {header}
        <div className='flex min-h-0 w-full flex-1'>
          <AppSidebar />
          <SidebarInset
            className={cn(
              '@container/content',
              'h-[calc(100svh-var(--app-header-height,0px))]',
              'min-h-0 overflow-hidden',
              'peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]'
            )}
          >
            <AnimatedOutlet />
          </SidebarInset>
        </div>
      </SidebarProvider>
    </LayoutProvider>
  )
}
