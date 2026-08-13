import { describe, expect, test } from 'bun:test'

import { translateMessageRef } from './message-ref'

describe('translateMessageRef metric presentation', () => {
  test('renders the safe collection execution fallback without raw diagnostics', () => {
    expect(
      translateMessageRef({
        code: 'COLLECTION_EXECUTION_FAILED',
        params: {},
        technical_detail: '',
      })
    ).toBe('采集任务执行失败，详细原因不可安全展示')
  })

  test.each([
    [
      'ALERT_CHANNEL_RESPONSE_TIME_HIGH',
      {
        site_id: '1',
        site_name: '生产站',
        threshold: '3000.0000000000',
        value: '3042.7500000000',
      },
      '站点 生产站 的渠道平均响应时间为 3,042.75 毫秒，达到或超过阈值 3,000 毫秒',
    ],
    [
      'ALERT_CHANNEL_BALANCE_LOW',
      {
        site_id: '1',
        site_name: '生产站',
        threshold: '10.5000000000',
        value: '0.0000000000',
      },
      '站点 生产站 的渠道余额总和为 0，达到或低于阈值 10.5',
    ],
    [
      'ALERT_CHANNEL_AVAILABILITY_LOW',
      {
        site_id: '1',
        site_name: '生产站',
        threshold: '0.8000000000',
        value: '0.7500000000',
      },
      '站点 生产站 的渠道可用率为 75%，达到或低于阈值 80%',
    ],
    [
      'ALERT_CPU_HIGH',
      {
        site_id: '1',
        target_name: 'api-1',
        target_type: 'instance',
        threshold: '85.0000000000',
        value: '91.2500000000',
      },
      'api-1 的 CPU 使用率为 91.25%，超过阈值 85%',
    ],
  ] as const)(
    'formats %s metric params by semantic unit',
    (code, params, expected) => {
      expect(translateMessageRef({ code, params, technical_detail: '' })).toBe(
        expected
      )
    }
  )
})
