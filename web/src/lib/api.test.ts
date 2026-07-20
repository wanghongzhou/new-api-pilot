import { afterEach, describe, expect, test } from 'bun:test'

import type { AxiosAdapter } from 'axios'

import {
  ApiError,
  api,
  createRequestId,
  getApiErrorTranslationKey,
  requestApiData,
  setAuthenticatedUserId,
} from './api'
import { isApiErrorCode } from './message-codes'
import { shouldRetryQuery } from './query-client'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
  setAuthenticatedUserId(null)
})

describe('shared API client', () => {
  test('adds a request ID and deduplicates concurrent GET requests', async () => {
    let calls = 0
    let releaseRequest: (() => void) | undefined
    const requestGate = new Promise<void>((resolve) => {
      releaseRequest = resolve
    })

    const adapter: AxiosAdapter = async (config) => {
      calls += 1
      await requestGate
      const requestId = config.headers.get('X-Request-ID')
      expect(requestId).toMatch(/^web_/)
      return {
        config,
        data: {
          success: true,
          message: '',
          code: '',
          data: { ok: true },
          request_id: 'req_test',
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    api.defaults.adapter = adapter

    const first = api.get('/api/test', { params: { p: 1 } })
    const second = api.get('/api/test', { params: { p: 1 } })
    releaseRequest?.()

    await Promise.all([first, second])
    expect(calls).toBe(1)
  })

  test('turns a failed success envelope into a typed business error', async () => {
    api.defaults.adapter = async (config) => ({
      config,
      data: {
        success: false,
        message: 'safe fallback',
        code: 'VALIDATION_ERROR',
        data: null,
        request_id: 'req_validation',
        field_errors: { name: 'required' },
      },
      headers: {},
      status: 200,
      statusText: 'OK',
    })

    const request = api.get('/api/validation')
    await expect(request).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      fieldErrors: { name: 'required' },
      kind: 'business',
      requestId: 'req_validation',
    })
  })

  test('retries only bounded network and server failures', () => {
    const serverError = new ApiError('server', {
      kind: 'http',
      status: 503,
      code: 'UPSTREAM_UNAVAILABLE',
      requestId: 'req_server',
      fieldErrors: null,
    })
    const forbidden = new ApiError('forbidden', {
      kind: 'http',
      status: 403,
      code: 'FORBIDDEN',
      requestId: 'req_forbidden',
      fieldErrors: null,
    })

    expect(shouldRetryQuery(0, serverError)).toBeTrue()
    expect(shouldRetryQuery(2, serverError)).toBeFalse()
    expect(shouldRetryQuery(0, forbidden)).toBeFalse()
    expect(getApiErrorTranslationKey(forbidden)).toBe('FORBIDDEN')
  })

  test('recognizes internal contract failures as API error codes', () => {
    const error = new ApiError('internal contract', {
      code: 'INTERNAL_CONTRACT_ERROR',
      fieldErrors: null,
      kind: 'http',
      requestId: 'req_internal_contract',
      status: 500,
    })

    expect(isApiErrorCode('INTERNAL_CONTRACT_ERROR')).toBeTrue()
    expect(getApiErrorTranslationKey(error)).toBe('INTERNAL_CONTRACT_ERROR')
  })

  test('creates an opaque browser request ID', () => {
    expect(createRequestId()).toMatch(/^web_[a-zA-Z0-9_-]+/)
  })

  test('sends the current user header only while authenticated', async () => {
    const received: Array<string | undefined> = []
    api.defaults.adapter = async (config) => {
      const value = config.headers.get('New-Api-User')
      received.push(typeof value === 'string' ? value : undefined)
      return {
        config,
        data: {
          success: true,
          message: '',
          code: '',
          data: null,
          request_id: 'req_header',
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }

    await api.post('/api/user/login', {})
    setAuthenticatedUserId('9007199254740993')
    await api.get('/api/user/self')
    setAuthenticatedUserId('9007199254740994')
    await api.get('/api/user/self?switched=1')
    setAuthenticatedUserId(null)
    await api.post('/api/user/login', {})

    expect(received).toEqual([
      undefined,
      '9007199254740993',
      '9007199254740994',
      undefined,
    ])
  })

  for (const mode of ['explicit signal', 'dedupe opt-out'] as const) {
    test(`guards ${mode} GET requests across user changes`, async () => {
      let releaseOld: (() => void) | undefined
      let markOldStarted: (() => void) | undefined
      const oldGate = new Promise<void>((resolve) => {
        releaseOld = resolve
      })
      const oldStarted = new Promise<void>((resolve) => {
        markOldStarted = resolve
      })
      const received: string[] = []
      api.defaults.adapter = async (config) => {
        const userId = String(config.headers.get('New-Api-User'))
        received.push(userId)
        if (userId === '9007199254740993') {
          markOldStarted?.()
          await oldGate
        }
        return {
          config,
          data: {
            success: true,
            message: '',
            code: '',
            data: { userId },
            request_id: `req_${userId}`,
          },
          headers: {},
          status: 200,
          statusText: 'OK',
        }
      }
      const config = () =>
        mode === 'explicit signal'
          ? { signal: new AbortController().signal }
          : { disableDedupe: true }

      setAuthenticatedUserId('9007199254740993')
      const oldRequest = requestApiData<{ userId: string }>({
        ...config(),
        method: 'get',
        url: '/api/session-scoped',
      })
      await oldStarted
      setAuthenticatedUserId('9007199254740994')
      const fresh = await requestApiData<{ userId: string }>({
        ...config(),
        method: 'get',
        url: '/api/session-scoped',
      })
      releaseOld?.()

      expect(fresh.userId).toBe('9007199254740994')
      await expect(oldRequest).rejects.toBeDefined()
      expect(received).toEqual(['9007199254740993', '9007199254740994'])
    })
  }
})
