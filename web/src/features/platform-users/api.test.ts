import { afterEach, describe, expect, test } from 'bun:test'

import { QueryClient } from '@tanstack/react-query'
import type { AxiosAdapter } from 'axios'

import { api, setAuthenticatedUserId } from '@/lib/api'

import { listPlatformUsers } from './api'
import type { PlatformUserPage } from './types'

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
  setAuthenticatedUserId(null)
})

describe('platform user API', () => {
  test('deduplicates concurrent feature GET requests', async () => {
    let calls = 0
    let release: (() => void) | undefined
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    const adapter: AxiosAdapter = async (config) => {
      calls += 1
      await gate
      return {
        config,
        data: {
          code: '',
          data: { items: [], page: 1, page_size: 20, total: 0 },
          message: '',
          request_id: 'req_users',
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    api.defaults.adapter = adapter
    setAuthenticatedUserId('9007199254740993')

    const first = listPlatformUsers({ p: 1, page_size: 20 })
    const second = listPlatformUsers({ p: 1, page_size: 20 })
    release?.()
    await Promise.all([first, second])

    expect(calls).toBe(1)
  })

  test('isolates a slow GET when the authenticated user changes', async () => {
    let calls = 0
    let releaseOld: (() => void) | undefined
    let markOldStarted: (() => void) | undefined
    const oldGate = new Promise<void>((resolve) => {
      releaseOld = resolve
    })
    const oldStarted = new Promise<void>((resolve) => {
      markOldStarted = resolve
    })
    const receivedHeaders: string[] = []
    api.defaults.adapter = async (config) => {
      calls += 1
      const userId = String(config.headers.get('New-Api-User'))
      receivedHeaders.push(userId)
      if (userId === '9007199254740993') {
        markOldStarted?.()
        await oldGate
      }
      return {
        config,
        data: {
          code: '',
          data: {
            items: [{ username: userId }],
            page: 1,
            page_size: 20,
            total: 1,
          },
          message: '',
          request_id: `req_${userId}`,
          success: true,
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const key = ['platform-users', 'same-url'] as const

    setAuthenticatedUserId('9007199254740993')
    const oldRequest = listPlatformUsers({ p: 1, page_size: 20 }).then(
      (data) => {
        client.setQueryData(key, data)
        return data
      }
    )
    await oldStarted

    setAuthenticatedUserId(null)
    client.clear()
    setAuthenticatedUserId('9007199254740994')
    const freshData = await client.fetchQuery({
      queryFn: () => listPlatformUsers({ p: 1, page_size: 20 }),
      queryKey: key,
    })
    expect(freshData.items[0]?.username).toBe('9007199254740994')

    releaseOld?.()
    await expect(oldRequest).rejects.toBeDefined()
    expect(client.getQueryData<PlatformUserPage>(key)).toEqual(freshData)
    expect(calls).toBe(2)
    expect(receivedHeaders).toEqual(['9007199254740993', '9007199254740994'])
  })
})
