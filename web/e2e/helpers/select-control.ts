import { expect, type Page } from '@playwright/test'

function openSelectTrigger(page: Page) {
  return page
    .locator('[role="combobox"][aria-expanded="true"]')
    .filter({ visible: true })
    .last()
}

async function listboxForTrigger(page: Page) {
  const trigger = openSelectTrigger(page)
  await expect(trigger).toBeVisible()
  const listboxId = await trigger.getAttribute('aria-controls')
  expect(listboxId).toBeTruthy()
  return page.locator(`[id=${JSON.stringify(listboxId)}]`)
}

export async function clickOpenSelectOption(page: Page, value: string) {
  await expect(await listboxForTrigger(page)).toBeVisible()

  let selectionAttempted = false
  await expect(async () => {
    const trigger = openSelectTrigger(page)
    if ((await trigger.count()) === 0) {
      expect(selectionAttempted).toBe(true)
      return
    }

    const listbox = await listboxForTrigger(page)
    const option = listbox.locator(
      `[role='option'][data-select-value=${JSON.stringify(value)}]`
    )
    selectionAttempted = true
    await option.click({ timeout: 3_000 })
    await expect(listbox).toBeHidden({ timeout: 1_000 })
  }).toPass({ timeout: 15_000 })
}
