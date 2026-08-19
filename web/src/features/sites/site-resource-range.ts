import { fromUnixSeconds } from '@/lib/dayjs'

export type SiteResourceGranularity = 'day' | 'hour' | 'minute'

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
    start: siteResourceRangeLimitStart(end.unix(), granularity),
  }
}

export function siteResourceRangeLimitStart(
  end: number,
  granularity: SiteResourceGranularity
): number {
  const value = fromUnixSeconds(end)
  if (granularity === 'minute') return value.subtract(1, 'day').unix()
  if (granularity === 'hour') return value.subtract(7, 'day').unix()
  let start = value.subtract(31, 'day')
  while (start.add(1, 'month').isBefore(value)) start = start.add(1, 'day')
  return start.unix()
}

export function siteResourceRangeLimitEnd(
  start: number,
  granularity: SiteResourceGranularity
): number {
  const value = fromUnixSeconds(start)
  if (granularity === 'minute') return value.add(1, 'day').unix()
  if (granularity === 'hour') return value.add(7, 'day').unix()
  return value.add(1, 'month').unix()
}

export function siteResourceRangeExceedsLimit(
  start: number,
  end: number,
  granularity: SiteResourceGranularity
): boolean {
  return end > siteResourceRangeLimitEnd(start, granularity)
}
