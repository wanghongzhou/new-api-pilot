import { expect, test } from '@playwright/test'

test('application shell boots without page errors', async ({ page }) => {
  const pageErrors: Error[] = []
  page.on('pageerror', (error) => pageErrors.push(error))

  await page.goto('/')

  await expect(page.locator('main')).toBeVisible()
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  expect(pageErrors).toEqual([])
})
