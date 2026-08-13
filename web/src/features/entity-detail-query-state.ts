import {
  getApiErrorTranslationKey,
  isRetryableApiError,
  normalizeApiError,
} from '@/lib/api'

export type EntityDetailFailureKind =
  | 'forbidden'
  | 'invalid-id'
  | 'not-found'
  | 'retryable'

export interface EntityDetailFailure {
  descriptionKey: string
  kind: EntityDetailFailureKind
  retryable: boolean
}

export function entityDetailFailure(
  validId: boolean,
  error: unknown,
  defaultDescriptionKey: string,
  invalidIdDescriptionKey: string
): EntityDetailFailure {
  if (!validId) {
    return {
      descriptionKey: invalidIdDescriptionKey,
      kind: 'invalid-id',
      retryable: false,
    }
  }

  const apiError = normalizeApiError(error)
  if (apiError.status === 404) {
    return { descriptionKey: 'NOT_FOUND', kind: 'not-found', retryable: false }
  }
  if (apiError.status === 403) {
    return {
      descriptionKey: getApiErrorTranslationKey(apiError),
      kind: 'forbidden',
      retryable: false,
    }
  }
  return {
    descriptionKey: isRetryableApiError(apiError)
      ? getApiErrorTranslationKey(apiError)
      : defaultDescriptionKey,
    kind: 'retryable',
    retryable: isRetryableApiError(apiError),
  }
}
