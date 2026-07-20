import { ApiError } from '@/lib/api'
import type { ApiResponse } from '@/lib/api-types'

import type { StatisticsExportDownload, StatisticsExportFormat } from './types'

type DownloadResponse = {
  data: unknown
  status: number
}

function isErrorEnvelope(value: unknown): value is ApiResponse<unknown> {
  if (!value || typeof value !== 'object') return false
  const envelope = value as Partial<ApiResponse<unknown>>
  return (
    envelope.success === false &&
    typeof envelope.code === 'string' &&
    typeof envelope.message === 'string' &&
    typeof envelope.request_id === 'string' &&
    'data' in envelope
  )
}

async function blobPayload(value: unknown): Promise<unknown> {
  if (!(value instanceof Blob)) return value
  try {
    return JSON.parse(await value.text()) as unknown
  } catch {
    return null
  }
}

export function safeExportFileName(
  value: string,
  format: StatisticsExportFormat,
  exportId: string
): string {
  const expectedSuffix = `.${format}`
  const safe =
    value.length > expectedSuffix.length &&
    value.length <= 255 &&
    !value.includes('/') &&
    !value.includes('\\') &&
    !value.includes('\0') &&
    value.toLowerCase().endsWith(expectedSuffix)
  return safe ? value : `statistics-export-${exportId}${expectedSuffix}`
}

export async function parseExportDownloadResponse(
  response: DownloadResponse,
  options: {
    exportId: string
    fileName: string
    format: StatisticsExportFormat
  }
): Promise<StatisticsExportDownload> {
  if (response.status >= 200 && response.status < 300) {
    if (!(response.data instanceof Blob)) {
      throw new ApiError('Unexpected export download response', {
        code: '',
        fieldErrors: null,
        kind: 'invalid-response',
        requestId: null,
        status: response.status,
      })
    }
    return {
      blob: response.data,
      fileName: safeExportFileName(
        options.fileName,
        options.format,
        options.exportId
      ),
    }
  }

  const payload = await blobPayload(response.data)
  if (isErrorEnvelope(payload)) {
    throw new ApiError(payload.message || 'Export download failed', {
      code: payload.code,
      fieldErrors: payload.field_errors ?? null,
      kind: 'http',
      messageRef: {
        code: payload.code,
        params: payload.params ?? {},
        technical_detail: '',
      },
      requestId: payload.request_id || null,
      status: response.status,
    })
  }
  throw new ApiError('Export download failed', {
    code: '',
    fieldErrors: null,
    kind: 'http',
    requestId: null,
    status: response.status,
  })
}
