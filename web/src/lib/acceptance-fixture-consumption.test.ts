import { describe, expect, test } from 'bun:test'

type F02Route = {
  id: string
  path: string
  response_file?: string
}

type F02Manifest = {
  fixture_id: string
  scenarios: Record<string, { routes: F02Route[] }>
}

type F04Fixture = {
  fixture_id: string
  scenarios: Array<{ id: string }>
}

const designUrl = (path: string) =>
  new URL(`../../../testdata/design/${path}`, import.meta.url)

async function jsonFixture<T>(path: string): Promise<T> {
  return (await Bun.file(designUrl(path)).json()) as T
}

describe('A89-A100 design fixture consumption', () => {
  test('A89-A92 load F02 responses and map F03-F05 operational contracts', async () => {
    const f02 = await jsonFixture<F02Manifest>('f02-upstream/manifest.json')
    const supported = f02.scenarios.supported.routes
    expect(f02.fixture_id).toBe('F02')

    const requiredRoutes = new Map([
      ['A89', supported.find((route) => route.id === 'realtime')],
      ['A90', supported.find((route) => route.id === 'users-page-1')],
      ['A91', supported.find((route) => route.id === 'channels-page-1')],
      [
        'A92',
        supported.find((route) => route.id === 'performance-summary-24h'),
      ],
    ])
    for (const [acceptanceId, route] of requiredRoutes) {
      expect(route, `${acceptanceId} route`).toBeDefined()
      expect(route?.response_file, `${acceptanceId} response`).toBeString()
      if (!route?.response_file) {
        throw new Error(`${acceptanceId} fixture response is missing`)
      }
      const response = await jsonFixture<Record<string, unknown>>(
        `f02-upstream/${route.response_file}`
      )
      expect(response).toBeObject()
    }

    const f03 = await Bun.file(designUrl('f03-statistics.sql')).text()
    expect(f03).toContain('usage_fact_hourly')
    expect(f03).toContain('collection_window')

    const f04 = await jsonFixture<F04Fixture>('f04-state-machines.json')
    expect(f04.fixture_id).toBe('F04')
    expect(f04.scenarios.map(({ id }) => id)).toContain(
      'performance_proxy_cache_fence'
    )

    const f05 = await Bun.file(designUrl('f05-ops-capacity.yaml')).text()
    expect(f05).toContain('fixture_id: F05')
    expect(f05).toContain('csv_utf8_bom: true')
    expect(f05).toContain('performance_proxy_cache:')
  })

  test('A93-A98 parse F06-F09 and bind every frozen scenario family', async () => {
    const f06 = await jsonFixture<{
      fixture_id: string
      topups: unknown[]
      redemptions: unknown[]
      snapshot_scenarios: string[]
    }>('f06-finance-operations.json')
    expect(f06.fixture_id).toBe('F06')
    expect(f06.topups).toHaveLength(1)
    expect(f06.redemptions).toHaveLength(1)
    expect(f06.snapshot_scenarios).toEqual(
      expect.arrayContaining([
        'complete',
        'total_drift',
        'maximum_id_drift',
        'duplicate_id',
        'over_100000',
        'missing',
        'reappear',
        'config_fence',
      ])
    )

    const f07 = await jsonFixture<{
      fixture_id: string
      tasks: unknown[]
      transitions: string[]
      polling_scenarios: string[]
    }>('f07-upstream-tasks.json')
    expect(f07.fixture_id).toBe('F07')
    expect(f07.tasks).toHaveLength(1)
    expect(f07.transitions).toContain('IN_PROGRESS->SUCCESS')
    expect(f07.polling_scenarios).toEqual(
      expect.arrayContaining(['known_unfinished_rescan', 'unfinished_retained'])
    )

    const f08 = await jsonFixture<{
      fixture_id: string
      models: unknown[]
      channels: unknown[]
      expected_exact_missing: string[]
      scenarios: string[]
    }>('f08-model-catalog.json')
    expect(f08.fixture_id).toBe('F08')
    expect(f08.models).toHaveLength(2)
    expect(f08.channels).toHaveLength(1)
    expect(f08.expected_exact_missing).toEqual(['unlisted-model'])
    expect(f08.scenarios).toContain('partial_snapshot_preserves_edges')

    const f09 = await jsonFixture<{
      fixture_id: string
      plans: Array<{ price_amount: string; total_amount: string }>
      scenarios: string[]
    }>('f09-subscription-plans.json')
    expect(f09.fixture_id).toBe('F09')
    expect(f09.plans[0]).toMatchObject({
      price_amount: '19.990000',
      total_amount: '9007199254740993',
    })
    expect(f09.scenarios).toEqual(
      expect.arrayContaining([
        'body_hard_cap',
        'duplicate_id',
        'config_fence',
        'complete_missing',
        'reappear',
        'sensitive_absence',
      ])
    )
  })

  test('A99-A100 directly parse F10-F11 and bind catalog/task scenarios', async () => {
    const f10 = await jsonFixture<{
      fixture_id: string
      groups: Array<{ group_name: string }>
      pricing: Array<{
        input_price: string
        model_name: string
        group_ratios: Record<string, string>
      }>
      scenarios: string[]
    }>('f10-pricing-groups.json')
    expect(f10.fixture_id).toBe('F10')
    expect(f10.groups.map(({ group_name }) => group_name)).toContain(
      'vip-zero-usage'
    )
    expect(f10.pricing[0]).toMatchObject({
      input_price: '0.0000025000',
      model_name: 'gpt-4o',
    })
    expect(f10.pricing[0]?.group_ratios['vip-zero-usage']).toBe('0.8500000000')
    expect(f10.scenarios).toEqual(
      expect.arrayContaining([
        'zero_usage_group_preserved',
        'exact_decimal_round_trip',
        'resource_independent_completeness',
        'no_remote_mutation',
      ])
    )

    const f11 = await jsonFixture<{
      fixture_id: string
      tasks: Array<{
        type: string
        status: string
        progress: { total: string; progress: number } | null
      }>
      scenarios: string[]
      upstream: { statuses: string[]; types: string[] }
    }>('f11-system-tasks.json')
    expect(f11.fixture_id).toBe('F11')
    expect(f11.tasks).toHaveLength(5)
    expect(f11.upstream.types).toHaveLength(5)
    expect(f11.upstream.statuses).toEqual([
      'pending',
      'running',
      'succeeded',
      'failed',
    ])
    expect(f11.tasks[0]?.progress).toMatchObject({
      progress: 44,
      total: '9007199254740993',
    })
    expect(f11.scenarios).toEqual(
      expect.arrayContaining([
        'list_limit_100_truncated_partial',
        'five_current_requests_only_no_per_task_n_plus_one',
        'typed_progress_result_only',
        'statistics_completeness_and_export',
        'no_remote_mutation_or_detail_proxy',
      ])
    )
  })
})
