import { describe, expect, test } from 'bun:test'

import { ApiError } from '@/lib/api'

import { entityDetailFailure } from './entity-detail-query-state'

function apiError(status: number | null, kind: 'http' | 'network' = 'http') {
  let code = ''
  if (status === 404) code = 'NOT_FOUND'
  else if (status === 403) code = 'FORBIDDEN'
  return new ApiError('failed', {
    code,
    fieldErrors: null,
    kind,
    requestId: null,
    status,
  })
}

describe('entityDetailFailure', () => {
  test('does not offer retry for invalid IDs, 404, or 403', () => {
    expect(
      entityDetailFailure(false, undefined, 'fallback', 'invalid')
    ).toEqual({
      descriptionKey: 'invalid',
      kind: 'invalid-id',
      retryable: false,
    })
    expect(
      entityDetailFailure(true, apiError(404), 'fallback', 'invalid')
    ).toEqual({
      descriptionKey: 'NOT_FOUND',
      kind: 'not-found',
      retryable: false,
    })
    expect(
      entityDetailFailure(true, apiError(403), 'fallback', 'invalid')
    ).toEqual({
      descriptionKey: 'FORBIDDEN',
      kind: 'forbidden',
      retryable: false,
    })
  })

  test('offers retry only for network and server failures', () => {
    expect(
      entityDetailFailure(
        true,
        apiError(null, 'network'),
        'fallback',
        'invalid'
      ).retryable
    ).toBe(true)
    expect(
      entityDetailFailure(true, apiError(503), 'fallback', 'invalid').retryable
    ).toBe(true)
    expect(
      entityDetailFailure(true, apiError(400), 'fallback', 'invalid')
    ).toEqual({
      descriptionKey: 'fallback',
      kind: 'retryable',
      retryable: false,
    })
  })
})
