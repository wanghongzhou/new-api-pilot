import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import { parseMetricString } from '@/lib/api-types'

import { systemTaskTruncationReasons, type SystemTaskStatistics } from './types'

describe('system task truncation contract', () => {
  test('covers every backend truncation reason', () => {
    expect(systemTaskTruncationReasons).toEqual([
      'source_limit',
      'id_gap',
      'source_limit_and_id_gap',
    ])
  })

  test('keeps truncation metadata on statistics responses', () => {
    const statistics = {
      as_of: null,
      data_error_code: '',
      data_status: 'complete',
      observed_count: parseMetricString('200'),
      site_breakdown: [],
      source_limit: parseMetricString('200'),
      status_breakdown: [],
      summary: {
        active: parseMetricString('0'),
        error_present: parseMetricString('0'),
        failed: parseMetricString('0'),
        succeeded: parseMetricString('0'),
        total: parseMetricString('0'),
      },
      truncated: true,
      truncation_reason: 'source_limit_and_id_gap',
      type_breakdown: [],
    } satisfies SystemTaskStatistics

    expect(statistics.truncation_reason).toBe('source_limit_and_id_gap')
  })

  test('renders a dedicated combined-reason message', async () => {
    const [page, locale] = await Promise.all([
      readFile(
        new URL('components/system-tasks-page.tsx', import.meta.url),
        'utf8'
      ),
      readFile(
        new URL('../../i18n/locales/zh-CN.json', import.meta.url),
        'utf8'
      ),
    ])

    expect(page).toContain('systemTasks.truncation.source_limit_and_id_gap')
    expect(locale).toContain('"systemTasks.truncation.source_limit_and_id_gap"')
  })
})
