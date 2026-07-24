function equalFilterValue(left: unknown, right: unknown): boolean {
  if (Array.isArray(left) && Array.isArray(right)) {
    return (
      left.length === right.length &&
      left.every((value, index) => Object.is(value, right[index]))
    )
  }
  return Object.is(left, right)
}

export function hasFilterChanges<T extends object, K extends keyof T>(
  current: T,
  baseline: T,
  keys: readonly K[]
): boolean {
  return keys.some((key) => !equalFilterValue(current[key], baseline[key]))
}
