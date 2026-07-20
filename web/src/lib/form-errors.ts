import type { FieldPath, FieldValues, UseFormSetError } from 'react-hook-form'

import { normalizeApiError } from './api'
import { translateMessageRef } from './message-ref'

export function applyApiFieldErrors<TValues extends FieldValues>(
  error: unknown,
  setError: UseFormSetError<TValues>,
  fieldMap: Readonly<Record<string, FieldPath<TValues> | 'root'>> = {}
): boolean {
  const apiError = normalizeApiError(error)
  if (!apiError.fieldErrors) return false

  let applied = false
  const message = translateMessageRef(apiError.messageRef, 'VALIDATION_ERROR')
  for (const field of Object.keys(apiError.fieldErrors)) {
    const target = fieldMap[field] ?? (field as FieldPath<TValues>)
    setError(target, {
      message,
      type: 'server',
    })
    applied = true
  }
  return applied
}
