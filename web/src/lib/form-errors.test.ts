import { describe, expect, test } from 'bun:test'

import type { UseFormSetError } from 'react-hook-form'

import { ApiError } from './api'
import { applyApiFieldErrors } from './form-errors'

type Values = { name: string; remark: string }

describe('API field error mapping', () => {
  test('uses the stable message ref and never displays server free text', () => {
    const applied: Array<{ field: string; message?: string; type?: string }> =
      []
    const setError = ((
      field: string,
      error: { message?: string; type?: string }
    ) => applied.push({ field, ...error })) as UseFormSetError<Values>
    const error = new ApiError('validation failed', {
      code: 'VALIDATION_ERROR',
      fieldErrors: {
        name: '该客户名称已经存在，请更换名称。',
        remark: ['备注第一项错误。', '备注第二项错误。'],
      },
      kind: 'http',
      requestId: 'req_field_error',
      status: 400,
    })

    expect(
      applyApiFieldErrors(error, setError, {
        name: 'name',
        remark: 'remark',
      })
    ).toBeTrue()
    expect(applied).toEqual([
      {
        field: 'name',
        message: '请检查标出的字段后重试',
        type: 'server',
      },
      {
        field: 'remark',
        message: '请检查标出的字段后重试',
        type: 'server',
      },
    ])
  })
})
