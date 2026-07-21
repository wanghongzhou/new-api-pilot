import { SidebarTrigger } from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'

type HeaderProps = React.HTMLAttributes<HTMLElement> & {
  showTrigger?: boolean
}

export function Header({
  className,
  children,
  showTrigger = true,
  ...props
}: HeaderProps) {
  return (
    <header
      className={cn(
        'sticky top-0 z-40 h-[var(--app-header-height,3rem)] w-full shrink-0 bg-transparent',
        className
      )}
      {...props}
    >
      <div className='flex h-full items-center gap-1.5 px-2 sm:gap-2 sm:px-3'>
        {showTrigger && <SidebarTrigger className='size-8' variant='ghost' />}
        {children}
      </div>
    </header>
  )
}
