const resourceGradientStops = [
  { hue: 145, value: 0 },
  { hue: 105, value: 55 },
  { hue: 80, value: 75 },
  { hue: 50, value: 90 },
  { hue: 25, value: 100 },
] as const

function formatHue(value: number) {
  return String(Number(value.toFixed(2)))
}

export function siteResourceColor(value: number | null): string | undefined {
  if (value == null || !Number.isFinite(value)) return undefined
  const bounded = Math.max(0, Math.min(100, value))
  for (let index = 0; index < resourceGradientStops.length - 1; index += 1) {
    const lower = resourceGradientStops[index]
    const upper = resourceGradientStops[index + 1]
    if (bounded <= upper.value) {
      const progress = (bounded - lower.value) / (upper.value - lower.value)
      const hue = lower.hue + (upper.hue - lower.hue) * progress
      return `oklch(var(--resource-metric-lightness) var(--resource-metric-chroma) ${formatHue(hue)})`
    }
  }
  return `oklch(var(--resource-metric-lightness) var(--resource-metric-chroma) 25)`
}
