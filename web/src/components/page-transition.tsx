/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Outlet, useRouterState } from '@tanstack/react-router'
import { motion, useReducedMotion, type Variants } from 'motion/react'
import type { ReactNode } from 'react'

import {
  CARD_ITEM_VARIANTS,
  CARD_STAGGER_VARIANTS,
  MOTION_TRANSITION,
  MOTION_VARIANTS,
  STAGGER_ITEM_VARIANTS,
  STAGGER_VARIANTS,
  TABLE_ROW_VARIANTS,
  TABLE_STAGGER_VARIANTS,
} from '@/lib/motion'

export function PageTransition({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  const shouldReduce = useReducedMotion()
  if (shouldReduce) return <div className={className}>{children}</div>
  return (
    <motion.div
      animate={MOTION_VARIANTS.pageEnter.animate}
      className={className}
      initial={MOTION_VARIANTS.pageEnter.initial}
      transition={MOTION_TRANSITION.default}
    >
      {children}
    </motion.div>
  )
}

export function AnimatedOutlet() {
  const shouldReduce = useReducedMotion()
  const routeKey = useRouterState({
    select: (state) => state.matches.at(-1)?.routeId ?? state.location.pathname,
  })

  if (shouldReduce) {
    return (
      <div className='flex min-h-0 flex-1 flex-col'>
        <Outlet />
      </div>
    )
  }

  return (
    <motion.div
      animate={MOTION_VARIANTS.pageEnter.animate}
      className='flex min-h-0 flex-1 flex-col'
      initial={MOTION_VARIANTS.pageEnter.initial}
      key={routeKey}
      transition={MOTION_TRANSITION.fast}
    >
      <Outlet />
    </motion.div>
  )
}

interface StaggerContainerProps {
  children: ReactNode
  className?: string
  variants?: Variants
}

export function StaggerContainer(props: StaggerContainerProps) {
  const shouldReduce = useReducedMotion()

  if (shouldReduce) {
    return <div className={props.className}>{props.children}</div>
  }

  return (
    <motion.div
      animate='animate'
      className={props.className}
      initial='initial'
      variants={props.variants ?? STAGGER_VARIANTS}
    >
      {props.children}
    </motion.div>
  )
}

interface StaggerItemProps {
  children: ReactNode
  className?: string
  variants?: Variants
}

export function StaggerItem(props: StaggerItemProps) {
  return (
    <motion.div
      className={props.className}
      variants={props.variants ?? STAGGER_ITEM_VARIANTS}
    >
      {props.children}
    </motion.div>
  )
}

export function TableStaggerContainer(props: StaggerContainerProps) {
  const shouldReduce = useReducedMotion()

  if (shouldReduce) {
    return props.children
  }

  return (
    <motion.tbody
      animate='animate'
      className={props.className}
      initial='initial'
      variants={TABLE_STAGGER_VARIANTS}
    >
      {props.children}
    </motion.tbody>
  )
}

export function TableStaggerRow(props: StaggerItemProps) {
  return (
    <motion.tr className={props.className} variants={TABLE_ROW_VARIANTS}>
      {props.children}
    </motion.tr>
  )
}

export function CardStaggerContainer(props: StaggerContainerProps) {
  const shouldReduce = useReducedMotion()

  if (shouldReduce) {
    return <div className={props.className}>{props.children}</div>
  }

  return (
    <motion.div
      animate='animate'
      className={props.className}
      initial='initial'
      variants={CARD_STAGGER_VARIANTS}
    >
      {props.children}
    </motion.div>
  )
}

export function CardStaggerItem(props: StaggerItemProps) {
  return (
    <motion.div className={props.className} variants={CARD_ITEM_VARIANTS}>
      {props.children}
    </motion.div>
  )
}

interface FadeInProps {
  children: ReactNode
  className?: string
  delay?: number
}

export function FadeIn(props: FadeInProps) {
  const shouldReduce = useReducedMotion()

  if (shouldReduce) {
    return <div className={props.className}>{props.children}</div>
  }

  return (
    <motion.div
      animate={MOTION_VARIANTS.fadeIn.animate}
      className={props.className}
      initial={MOTION_VARIANTS.fadeIn.initial}
      transition={{
        ...MOTION_TRANSITION.default,
        delay: props.delay,
      }}
    >
      {props.children}
    </motion.div>
  )
}
