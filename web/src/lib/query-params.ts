function hasQueryContent(value: unknown): boolean {
  return value !== undefined && value !== null && value !== ''
}

function compactArray(values: readonly unknown[]): unknown[] {
  return values.filter(hasQueryContent)
}

export function compactQueryRecord(
  values: Readonly<Record<string, unknown>>
): Record<string, unknown> {
  const result: Record<string, unknown> = {}

  for (const [key, value] of Object.entries(values)) {
    if (key === '') continue
    if (Array.isArray(value)) {
      const compacted = compactArray(value)
      if (compacted.length > 0) result[key] = compacted
      continue
    }
    if (hasQueryContent(value)) result[key] = value
  }

  return result
}

function compactURLSearchParams(values: URLSearchParams): URLSearchParams {
  const result = new URLSearchParams()
  for (const [key, value] of values) {
    if (key !== '' && value !== '') result.append(key, value)
  }
  return result
}

function isPlainRecord(value: object): value is Record<string, unknown> {
  const prototype = Object.getPrototypeOf(value)
  return prototype === Object.prototype || prototype === null
}

export function compactRequestParams(params: unknown): unknown {
  if (params instanceof URLSearchParams) {
    const compacted = compactURLSearchParams(params)
    return compacted.size > 0 ? compacted : undefined
  }
  if (params && typeof params === 'object' && isPlainRecord(params)) {
    const compacted = compactQueryRecord(params)
    return Object.keys(compacted).length > 0 ? compacted : undefined
  }
  return params
}

export function compactQueryUrl(url: string): string {
  const hashIndex = url.indexOf('#')
  const hash = hashIndex >= 0 ? url.slice(hashIndex) : ''
  const withoutHash = hashIndex >= 0 ? url.slice(0, hashIndex) : url
  const queryIndex = withoutHash.indexOf('?')
  if (queryIndex < 0) return url

  const path = withoutHash.slice(0, queryIndex)
  const compacted = compactURLSearchParams(
    new URLSearchParams(withoutHash.slice(queryIndex + 1))
  )
  const query = compacted.toString()
  return `${path}${query ? `?${query}` : ''}${hash}`
}
