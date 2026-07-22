import { expect, type Page } from '@playwright/test'

export async function clickOpenSelectOption(page: Page, value: string) {
  const listbox = page.getByRole('listbox').filter({ visible: true }).last()
  await expect(listbox).toBeVisible()
  const listboxId = await listbox.getAttribute('id')
  expect(listboxId).toBeTruthy()
  await page
    .locator("button[aria-label='Open TanStack Router Devtools']")
    .evaluateAll((buttons) => {
      for (const button of buttons) {
        ;(button as HTMLElement).style.pointerEvents = 'none'
      }
    })
  await listbox
    .locator(`[role='option'][data-select-value=${JSON.stringify(value)}]`)
    .click()
  await expect(page.locator(`[id=${JSON.stringify(listboxId)}]`)).toBeHidden()
}
