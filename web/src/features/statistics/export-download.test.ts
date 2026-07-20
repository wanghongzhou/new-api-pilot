import { describe, expect, test } from 'bun:test'

import { ApiError } from '@/lib/api'

import {
  parseExportDownloadResponse,
  safeExportFileName,
} from './export-download'

describe('export download safety', () => {
  test('uses only a bounded basename with the expected extension', () => {
    expect(safeExportFileName('global-statistics.xlsx', 'xlsx', '71')).toBe(
      'global-statistics.xlsx'
    )
    expect(safeExportFileName('../secret.csv', 'csv', '72')).toBe(
      'statistics-export-72.csv'
    )
    expect(safeExportFileName('wrong.xlsx', 'csv', '73')).toBe(
      'statistics-export-73.csv'
    )
  })

  test('returns a Blob with a safe download name on success', async () => {
    const blob = new Blob(['a,b\n1,2'], { type: 'text/csv' })
    await expect(
      parseExportDownloadResponse(
        { data: blob, status: 200 },
        { exportId: '74', fileName: 'safe.csv', format: 'csv' }
      )
    ).resolves.toEqual({ blob, fileName: 'safe.csv' })
  })

  test('preserves the stable 410 MessageRef without exposing raw text', async () => {
    const blob = new Blob(
      [
        JSON.stringify({
          code: 'EXPORT_FILE_MISSING',
          data: null,
          message: 'internal path must stay hidden',
          params: { export_id: '75' },
          request_id: 'req_missing',
          success: false,
        }),
      ],
      { type: 'application/json' }
    )
    let caught: unknown
    try {
      await parseExportDownloadResponse(
        { data: blob, status: 410 },
        { exportId: '75', fileName: 'safe.csv', format: 'csv' }
      )
    } catch (error) {
      caught = error
    }
    expect(caught).toBeInstanceOf(ApiError)
    const apiError = caught as ApiError
    expect(apiError.status).toBe(410)
    expect(apiError.code).toBe('EXPORT_FILE_MISSING')
    expect(apiError.messageRef?.params).toEqual({ export_id: '75' })
    expect(apiError.messageRef?.technical_detail).toBe('')
  })

  test('turns a non-JSON 410 body into a generic error', async () => {
    let caught: unknown
    try {
      await parseExportDownloadResponse(
        { data: new Blob(['private upstream response']), status: 410 },
        { exportId: '76', fileName: 'safe.csv', format: 'csv' }
      )
    } catch (error) {
      caught = error
    }
    expect(caught).toBeInstanceOf(ApiError)
    const apiError = caught as ApiError
    expect(apiError.code).toBe('')
    expect(apiError.message).not.toContain('private upstream response')
  })
})
