import axios, {
  AxiosError,
  AxiosHeaders,
  CanceledError,
  type AxiosRequestConfig,
  type AxiosResponse,
} from 'axios'

import type { ApiResponse, FieldErrors } from './api-types'
import { isStableI18nCode } from './message-codes'
import type { UnknownMessageRef } from './message-ref'

declare module 'axios' {
  export interface AxiosRequestConfig {
    disableDedupe?: boolean
    skipAuthHandling?: boolean
  }
}

export type ApiErrorKind = 'business' | 'http' | 'network' | 'invalid-response'

export interface ApiErrorOptions {
  kind: ApiErrorKind
  status: number | null
  code: string
  requestId: string | null
  fieldErrors: FieldErrors | null
  messageRef?: UnknownMessageRef | null
  cause?: unknown
}

export class ApiError extends Error {
  readonly kind: ApiErrorKind
  readonly status: number | null
  readonly code: string
  readonly requestId: string | null
  readonly fieldErrors: FieldErrors | null
  readonly messageRef: UnknownMessageRef | null

  constructor(message: string, options: ApiErrorOptions) {
    super(message, { cause: options.cause })
    this.name = 'ApiError'
    this.kind = options.kind
    this.status = options.status
    this.code = options.code
    this.requestId = options.requestId
    this.fieldErrors = options.fieldErrors
    this.messageRef =
      options.messageRef ??
      (isStableI18nCode(options.code)
        ? { code: options.code, params: {}, technical_detail: '' }
        : null)
  }
}

type UnauthorizedHandler = (error: ApiError) => void

let unauthorizedHandler: UnauthorizedHandler | null = null
let authenticatedUserId: string | null = null
let authenticationEpoch = 0

interface InFlightGet {
  controller: AbortController
  epoch: number
  promise: Promise<AxiosResponse<unknown>>
}

const inFlightGets = new Map<string, InFlightGet>()
const activeGets = new Set<InFlightGet>()

export function setAuthenticatedUserId(userId: string | null): void {
  if (authenticatedUserId === userId) return
  authenticatedUserId = userId
  authenticationEpoch += 1
  for (const request of activeGets) request.controller.abort()
  inFlightGets.clear()
}

export function setUnauthorizedHandler(
  handler: UnauthorizedHandler | null
): () => void {
  unauthorizedHandler = handler
  return () => {
    if (unauthorizedHandler === handler) unauthorizedHandler = null
  }
}

export function createRequestId(): string {
  const randomId = globalThis.crypto?.randomUUID?.()
  if (randomId) return `web_${randomId}`

  const randomPart = Math.random().toString(36).slice(2)
  return `web_${Date.now().toString(36)}_${randomPart}`
}

export const api = axios.create({
  baseURL: '',
  timeout: 30_000,
  withCredentials: true,
  headers: {
    Accept: 'application/json',
    'Cache-Control': 'no-store',
  },
})

api.interceptors.request.use((config) => {
  const headers = AxiosHeaders.from(config.headers)
  if (authenticatedUserId) {
    headers.set('New-Api-User', authenticatedUserId)
  } else {
    headers.delete('New-Api-User')
  }
  if (!headers.has('X-Request-ID')) {
    headers.set('X-Request-ID', createRequestId())
  }
  config.headers = headers
  return config
})

function isApiResponse(value: unknown): value is ApiResponse<unknown> {
  if (!value || typeof value !== 'object') return false
  const response = value as Partial<ApiResponse<unknown>>
  return (
    typeof response.success === 'boolean' &&
    typeof response.message === 'string' &&
    typeof response.code === 'string' &&
    typeof response.request_id === 'string' &&
    'data' in response
  )
}

function apiErrorFromEnvelope(
  response: ApiResponse<unknown>,
  status: number,
  cause?: unknown
): ApiError {
  return new ApiError(response.message || 'API request failed', {
    kind: status >= 400 ? 'http' : 'business',
    status,
    code: response.code,
    requestId: response.request_id || null,
    fieldErrors: response.field_errors ?? null,
    messageRef: response.code
      ? {
          code: response.code,
          params: response.params ?? {},
          technical_detail: '',
        }
      : null,
    cause,
  })
}

export function normalizeApiError(error: unknown): ApiError {
  if (error instanceof ApiError) return error

  if (error instanceof AxiosError) {
    const status = error.response?.status ?? null
    const payload: unknown = error.response?.data
    const responseRequestId = error.response?.headers?.['x-request-id']
    const requestRequestId = AxiosHeaders.from(error.config?.headers).get(
      'X-Request-ID'
    )
    let requestId: string | null = null
    if (typeof responseRequestId === 'string') {
      requestId = responseRequestId
    } else if (typeof requestRequestId === 'string') {
      requestId = requestRequestId
    }
    if (status != null && isApiResponse(payload)) {
      return apiErrorFromEnvelope(payload, status, error)
    }

    if (status == null) {
      return new ApiError(error.message || 'Network request failed', {
        kind: 'network',
        status: null,
        code: '',
        requestId,
        fieldErrors: null,
        cause: error,
      })
    }

    return new ApiError(error.message || 'HTTP request failed', {
      kind: 'http',
      status,
      code: '',
      requestId,
      fieldErrors: null,
      cause: error,
    })
  }

  return new ApiError(
    error instanceof Error ? error.message : 'Unexpected server response',
    {
      kind: 'invalid-response',
      status: null,
      code: '',
      requestId: null,
      fieldErrors: null,
      cause: error,
    }
  )
}

api.interceptors.response.use(
  (response) => {
    if (isApiResponse(response.data) && !response.data.success) {
      throw apiErrorFromEnvelope(response.data, response.status)
    }
    return response
  },
  (error: unknown) => {
    const apiError = normalizeApiError(error)
    const requestConfig = error instanceof AxiosError ? error.config : undefined
    if (
      apiError.status === 401 &&
      !requestConfig?.skipAuthHandling &&
      unauthorizedHandler
    ) {
      unauthorizedHandler(apiError)
    }
    return Promise.reject(apiError)
  }
)

const originalGet = api.get.bind(api)

function createGetDedupeKey(url: string, config: AxiosRequestConfig): string {
  const sourceHeaders = config.headers as Parameters<
    typeof AxiosHeaders.from
  >[0]
  const headers = Object.entries(AxiosHeaders.from(sourceHeaders).toJSON())
    .map(([key, value]) => [key.toLowerCase(), String(value)] as const)
    .sort(([left], [right]) => left.localeCompare(right))
  return JSON.stringify({
    authenticationEpoch,
    authenticatedUserId,
    headers,
    responseType: config.responseType ?? 'json',
    skipAuthHandling: config.skipAuthHandling ?? false,
    timeout: config.timeout ?? api.defaults.timeout,
    uri: api.getUri({ ...config, method: 'get', url }),
    withCredentials: config.withCredentials ?? api.defaults.withCredentials,
  })
}

api.get = ((url: string, config: AxiosRequestConfig = {}) => {
  const dedupeEnabled = !config.disableDedupe && !config.signal
  const key = dedupeEnabled ? createGetDedupeKey(url, config) : null
  const existing = key ? inFlightGets.get(key) : undefined
  if (existing) return existing.promise

  const requestEpoch = authenticationEpoch
  const controller = new AbortController()
  const externalSignal = config.signal
  const abortFromExternal = () => controller.abort()
  if (externalSignal?.aborted) abortFromExternal()
  else {
    externalSignal?.addEventListener?.('abort', abortFromExternal, {
      once: true,
    })
  }
  const request = originalGet(url, {
    ...config,
    signal: controller.signal,
  }) as Promise<AxiosResponse<unknown>>
  const guardedRequest = request.then(
    (response) => {
      if (requestEpoch !== authenticationEpoch) {
        throw new CanceledError('Authentication session changed')
      }
      return response
    },
    (error: unknown) => {
      if (requestEpoch !== authenticationEpoch) {
        throw new CanceledError('Authentication session changed')
      }
      throw error
    }
  )
  const entry: InFlightGet = {
    controller,
    epoch: requestEpoch,
    promise: guardedRequest,
  }
  activeGets.add(entry)
  if (key) inFlightGets.set(key, entry)
  const clear = () => {
    activeGets.delete(entry)
    if (key && inFlightGets.get(key) === entry) inFlightGets.delete(key)
    externalSignal?.removeEventListener?.('abort', abortFromExternal)
  }
  void guardedRequest.then(clear, clear)
  return guardedRequest
}) as typeof api.get

export async function requestApi<TData>(
  config: AxiosRequestConfig
): Promise<ApiResponse<TData>> {
  const method = (config.method ?? 'get').toLowerCase()
  const response =
    method === 'get'
      ? await api.get<ApiResponse<TData>>(config.url ?? '', config)
      : await api.request<ApiResponse<TData>>(config)
  if (!isApiResponse(response.data)) {
    throw new ApiError('Unexpected server response', {
      kind: 'invalid-response',
      status: response.status,
      code: '',
      requestId: null,
      fieldErrors: null,
    })
  }
  return response.data
}

export async function requestApiData<TData>(
  config: AxiosRequestConfig
): Promise<TData> {
  const response = await requestApi<TData>(config)
  return response.data
}

export function getApiErrorTranslationKey(error: unknown): string {
  const apiError = normalizeApiError(error)
  if (isStableI18nCode(apiError.code)) return apiError.code
  if (apiError.kind === 'network') return 'Network request failed'
  if (apiError.status === 401) return 'Session expired'
  if (apiError.status === 403) return 'You do not have permission'
  if (apiError.kind === 'invalid-response') return 'Unexpected server response'
  return 'Request failed'
}

export function isRetryableApiError(error: unknown): boolean {
  const apiError = normalizeApiError(error)
  return apiError.kind === 'network' || (apiError.status ?? 0) >= 500
}
