import { stringifySearchWith } from '@tanstack/react-router'

const positiveDecimalIntegerPattern = /^[1-9]\d*$/
const idSearchKeyPattern = /ids?$/i

function parseSearchValue(key: string, value: string): unknown {
  if (
    idSearchKeyPattern.test(key) &&
    positiveDecimalIntegerPattern.test(value)
  ) {
    return value
  }
  try {
    return JSON.parse(value)
  } catch {
    return value
  }
}

function probeSerializableString(value: string): unknown {
  if (positiveDecimalIntegerPattern.test(value)) {
    throw new SyntaxError('Preserve positive decimal strings')
  }
  return JSON.parse(value)
}

export function parseRouterSearch(search: string): Record<string, unknown> {
  const params = new URLSearchParams(
    search.startsWith('?') ? search.slice(1) : search
  )
  const result: Record<string, unknown> = Object.create(null)

  for (const [key, rawValue] of params) {
    const value = parseSearchValue(key, rawValue)
    const previous = result[key]
    if (previous === undefined) {
      result[key] = value
    } else if (Array.isArray(previous)) {
      previous.push(value)
    } else {
      result[key] = [previous, value]
    }
  }

  return result
}

export const stringifyRouterSearch = stringifySearchWith(
  JSON.stringify,
  probeSerializableString
)
