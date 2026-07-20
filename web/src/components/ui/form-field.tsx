import { cloneElement, type ReactElement, type ReactNode } from 'react'

import { cn } from '@/lib/utils'

export interface FormFieldProps {
  children: ReactElement
  className?: string
  description?: ReactNode
  error?: ReactNode
  htmlFor: string
  label: ReactNode
  required?: boolean
}

interface AccessibleControlProps {
  'aria-describedby'?: string
  'aria-errormessage'?: string
}

export function FormField({
  children,
  className,
  description,
  error,
  htmlFor,
  label,
  required = false,
}: FormFieldProps) {
  const descriptionId = description ? `${htmlFor}-description` : undefined
  const errorId = error ? `${htmlFor}-error` : undefined
  const controlElement = children as ReactElement<AccessibleControlProps>
  const childProps = controlElement.props
  const describedBy = [
    typeof childProps['aria-describedby'] === 'string'
      ? childProps['aria-describedby']
      : undefined,
    descriptionId,
    errorId,
  ]
    .filter(Boolean)
    .join(' ')
  const control = cloneElement(controlElement, {
    'aria-describedby': describedBy || undefined,
    'aria-errormessage': errorId,
  })

  return (
    <div className={cn('grid gap-1.5', className)}>
      <label className='text-sm font-medium' htmlFor={htmlFor}>
        {label}
        {required && <span aria-hidden='true'> *</span>}
      </label>
      {control}
      {description && (
        <p className='text-muted-foreground text-xs' id={descriptionId}>
          {description}
        </p>
      )}
      {error && (
        <p className='text-destructive text-xs' id={errorId} role='alert'>
          {error}
        </p>
      )}
    </div>
  )
}
