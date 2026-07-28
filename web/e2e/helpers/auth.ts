import type { Page } from '@playwright/test'

export async function mockAuthenticatedShell(
  page: Page,
  firingCount = 0
): Promise<void> {
  await page.route('**/api/alerts/summary', async (route) => {
    await route.fulfill({
      json: {
        code: '',
        data: {
          critical_count: 0,
          firing_count: firingCount,
          resolved_today_count: 0,
          updated_at: 1_784_000_000,
          warning_count: firingCount,
        },
        message: '',
        request_id: 'req_e2e_alert_summary',
        success: true,
      },
    })
  })
}
