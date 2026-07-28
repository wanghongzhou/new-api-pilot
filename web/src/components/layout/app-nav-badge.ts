export type AlertNavBadgeState =
  | { kind: 'count'; text: string }
  | { kind: 'stale'; text: string }
  | { kind: 'unknown'; text: '?' }
  | { kind: 'none'; text: null }

export function resolveAlertNavBadge(
  count: number | undefined,
  failed: boolean
): AlertNavBadgeState {
  if (count != null && Number.isInteger(count) && count > 0) {
    return {
      kind: failed ? 'stale' : 'count',
      text: count > 99 ? '99+' : String(count),
    }
  }
  if (failed) {
    return count == null
      ? { kind: 'unknown', text: '?' }
      : { kind: 'stale', text: '!' }
  }
  return { kind: 'none', text: null }
}
