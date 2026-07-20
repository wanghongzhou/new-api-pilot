import { Loading03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'

import { cn } from '@/lib/utils'

export function Spinner({ className }: { className?: string }) {
  return (
    <HugeiconsIcon
      aria-hidden='true'
      className={cn('size-4 animate-spin', className)}
      icon={Loading03Icon}
      strokeWidth={2}
    />
  )
}
