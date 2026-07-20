import dayjs, { type Dayjs } from 'dayjs'
import customParseFormat from 'dayjs/plugin/customParseFormat'
import relativeTime from 'dayjs/plugin/relativeTime'
import timezone from 'dayjs/plugin/timezone'
import utc from 'dayjs/plugin/utc'

dayjs.extend(customParseFormat)
dayjs.extend(relativeTime)
dayjs.extend(utc)
dayjs.extend(timezone)

export const BEIJING_TIMEZONE = 'Asia/Shanghai'

dayjs.tz.setDefault(BEIJING_TIMEZONE)

export type TimeGranularity = 'hour' | 'day' | 'month' | 'year'
export type CalendarInput = Dayjs | Date | string

export interface TimestampRange {
  startTimestamp: number
  endTimestamp: number
}

export function fromUnixSeconds(timestamp: number): Dayjs {
  return dayjs.unix(timestamp).tz(BEIJING_TIMEZONE)
}

export function toUnixSeconds(value: CalendarInput): number {
  return dayjs(value).unix()
}

export function toBeijingTime(value: CalendarInput): Dayjs {
  return dayjs(value).tz(BEIJING_TIMEZONE)
}

export function beijingNaturalDayRange(value: CalendarInput): TimestampRange {
  const start = toBeijingTime(value).startOf('day')
  return {
    startTimestamp: start.unix(),
    endTimestamp: start.add(1, 'day').unix(),
  }
}

export function parseBeijingNaturalDay(value: string): TimestampRange {
  const parsed = dayjs.tz(value, 'YYYY-MM-DD', BEIJING_TIMEZONE)
  if (!parsed.isValid() || parsed.format('YYYY-MM-DD') !== value) {
    throw new RangeError('Expected a valid YYYY-MM-DD date')
  }
  return beijingNaturalDayRange(parsed)
}

export function alignToBeijingHour(timestamp: number): number {
  return fromUnixSeconds(timestamp).startOf('hour').unix()
}

export function isBeijingBucketAligned(
  timestamp: number,
  granularity: TimeGranularity
): boolean {
  const value = fromUnixSeconds(timestamp)
  return value.startOf(granularity).unix() === timestamp
}

export function formatBeijingTimestamp(
  timestamp: number,
  granularity: TimeGranularity = 'hour'
): string {
  const value = fromUnixSeconds(timestamp)
  switch (granularity) {
    case 'hour':
      return value.format('YYYY-MM-DD HH:00')
    case 'day':
      return value.format('YYYY-MM-DD')
    case 'month':
      return value.format('YYYY-MM')
    case 'year':
      return value.format('YYYY')
  }
}

export { dayjs }
