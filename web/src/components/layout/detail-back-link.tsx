import type { ComponentProps } from 'react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type DetailBackLinkProps = Omit<
  ComponentProps<typeof Button>,
  'size' | 'variant'
>

export function DetailBackLink({ className, ...props }: DetailBackLinkProps) {
  return (
    <Button
      {...props}
      className={cn(
        'text-muted-foreground hover:text-foreground w-fit justify-start px-1 font-normal',
        className
      )}
      size='default'
      variant='ghost'
    />
  )
}
