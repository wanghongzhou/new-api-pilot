import { Dialog } from '@base-ui/react/dialog'
import { useMutation } from '@tanstack/react-query'
import { Outlet, useRouter } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { logout } from '@/features/auth/api'
import { clearAuthenticatedSession } from '@/features/auth/session'
import { useAuthStore } from '@/stores/auth-store'

import { AppHeader } from './app-header'
import { AppSidebar } from './app-sidebar'

export function AuthenticatedLayout() {
  const { t } = useTranslation()
  const router = useRouter()
  const user = useAuthStore((state) => state.user)
  const [mobileOpen, setMobileOpen] = useState(false)
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
      onOpenMenu={() => setMobileOpen(true)}
      showMenu={!user.must_change_password}
      user={user}
    />
  )

  if (user.must_change_password) {
    return (
      <div className='bg-background flex h-svh flex-col'>
        {header}
        <div className='min-h-0 flex-1 overflow-y-auto'>
          <Outlet />
        </div>
      </div>
    )
  }

  return (
    <div className='bg-background flex h-svh flex-col overflow-hidden'>
      <a
        className='bg-primary text-primary-foreground focus-visible:ring-ring/60 fixed -top-20 left-2 z-[100] inline-flex min-h-10 items-center rounded-md px-3 py-2 text-sm outline-none focus:top-2 focus-visible:ring-2'
        href='#main-content'
      >
        {t('Skip to main content')}
      </a>
      {header}
      <div className='flex min-h-0 flex-1 overflow-hidden'>
        <aside className='bg-sidebar hidden w-64 shrink-0 border-r lg:block'>
          <AppSidebar user={user} />
        </aside>
        <div className='flex h-full max-h-full min-h-0 min-w-0 flex-1 flex-col'>
          <Outlet />
        </div>
      </div>
      <Dialog.Root onOpenChange={setMobileOpen} open={mobileOpen}>
        <Dialog.Portal>
          <Dialog.Backdrop className='fixed inset-0 z-50 bg-black/35 lg:hidden' />
          <Dialog.Popup
            aria-label={t('Primary navigation')}
            className='bg-sidebar text-sidebar-foreground fixed inset-y-0 left-0 z-50 w-[min(18rem,85vw)] border-r shadow-xl outline-none lg:hidden'
          >
            <AppSidebar onNavigate={() => setMobileOpen(false)} user={user} />
          </Dialog.Popup>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  )
}
