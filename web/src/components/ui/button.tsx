import { Button as BaseButton } from '@base-ui/react/button'
import { cva, type VariantProps } from 'class-variance-authority'
import { isValidElement } from 'react'

import { cn } from '@/lib/utils'

export const buttonVariants = cva(
  'inline-flex min-h-10 shrink-0 items-center justify-center gap-2 rounded-md border border-transparent px-3 text-sm font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60 disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        primary: 'bg-primary text-primary-foreground hover:bg-primary/90',
        secondary:
          'bg-secondary text-secondary-foreground hover:bg-secondary/75',
        outline: 'border-border bg-background text-foreground hover:bg-muted',
        ghost: 'text-foreground hover:bg-muted',
        destructive:
          'bg-destructive text-destructive-foreground hover:bg-destructive/90',
      },
      size: {
        default: 'h-10',
        sm: 'h-10 px-2.5 text-xs',
        icon: 'size-10 p-0',
      },
    },
    defaultVariants: {
      variant: 'primary',
      size: 'default',
    },
  }
)

function rendersNativeButton(render: BaseButton.Props['render']): boolean {
  return !render || !isValidElement(render) || render.type === 'button'
}

export function Button({
  className,
  nativeButton,
  render,
  size,
  variant,
  ...props
}: BaseButton.Props & VariantProps<typeof buttonVariants>) {
  return (
    <BaseButton
      className={cn(buttonVariants({ size, variant }), className)}
      nativeButton={nativeButton ?? rendersNativeButton(render)}
      render={render}
      {...props}
    />
  )
}
