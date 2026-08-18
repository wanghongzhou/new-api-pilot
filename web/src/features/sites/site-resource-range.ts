import { fromUnixSeconds } from '@/lib/dayjs'

export type SiteResourceGranularity = 'day' | 'hour' | 'minute'

const defaultRangeCounts: Record<SiteResourceGranularity, number> = {
  day: 30,
  hour: 24,
  minute: 60,
}

export function alignSiteResourceTimestamp(
  timestamp: number,
  granularity: SiteResourceGranularity
): number {
  return fromUnixSeconds(timestamp).startOf(granularity).unix()
}

export function defaultSiteResourceRange(
  granularity: SiteResourceGranularity,
  now = Date.now() / 1000
): { end: number; start: number } {
  const end = fromUnixSeconds(now).startOf(granularity)
  return {
    end: end.unix(),
    start: end.subtract(defaultRangeCounts[granularity], granularity).unix(),
  }
}
