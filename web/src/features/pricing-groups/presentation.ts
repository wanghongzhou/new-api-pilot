export const PRICING_DISCLOSURE_ITEM_LIMIT = 6
export const PRICING_DISCLOSURE_TEXT_LIMIT = 160

export function visiblePricingValues<T>(
  values: readonly T[],
  expanded: boolean,
  limit = PRICING_DISCLOSURE_ITEM_LIMIT
) {
  return expanded ? values : values.slice(0, limit)
}

export function hiddenPricingValueCount(
  values: readonly unknown[],
  expanded: boolean,
  limit = PRICING_DISCLOSURE_ITEM_LIMIT
) {
  return expanded ? 0 : Math.max(0, values.length - limit)
}

export function keyedPricingValues(values: readonly string[]) {
  const occurrences = new Map<string, number>()
  return values.map((value) => {
    const occurrence = (occurrences.get(value) ?? 0) + 1
    occurrences.set(value, occurrence)
    return { key: `${value}\u0000${occurrence}`, value }
  })
}

export function visiblePricingText(
  value: string,
  expanded: boolean,
  limit = PRICING_DISCLOSURE_TEXT_LIMIT
) {
  if (expanded || value.length <= limit) return value
  return `${value.slice(0, limit)}…`
}
