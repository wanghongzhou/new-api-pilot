import type { LogType } from './types'

export function isConsumptionLogType(type: LogType): boolean {
  return type === 2
}
