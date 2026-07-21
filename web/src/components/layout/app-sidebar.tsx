import { AnimatePresence, motion, useReducedMotion } from 'motion/react'

import { Sidebar, SidebarContent, SidebarRail } from '@/components/ui/sidebar'
import { useLayout } from '@/context/layout-provider'
import { MOTION_TRANSITION, MOTION_VARIANTS } from '@/lib/motion'

import { AppNav } from './app-nav'

export function AppSidebar() {
  const { collapsible, variant } = useLayout()
  const shouldReduce = useReducedMotion()

  return (
    <Sidebar collapsible={collapsible} variant={variant}>
      <SidebarContent className='py-2'>
        <AnimatePresence initial={false} mode='wait'>
          <motion.div
            animate={MOTION_VARIANTS.sidebarSlide.animate}
            className='flex flex-col'
            exit={shouldReduce ? undefined : MOTION_VARIANTS.sidebarSlide.exit}
            initial={
              shouldReduce ? false : MOTION_VARIANTS.sidebarSlide.initial
            }
            key='main-navigation'
            transition={MOTION_TRANSITION.fast}
          >
            <AppNav />
          </motion.div>
        </AnimatePresence>
      </SidebarContent>
      <SidebarRail />
    </Sidebar>
  )
}
