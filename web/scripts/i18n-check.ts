import { readdir, readFile } from 'node:fs/promises'
import { dirname, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  findBootstrapHtmlViolations,
  findDuplicateJsonKeys,
  findHardcodedVisibleText,
  findMissingTranslationKeys,
  findTranslationKeyUsage,
  findUnknownDynamicRegistries,
  parseDynamicI18nRegistries,
  type DynamicRegistryUsage,
  type TranslationKeyUsage,
} from '../src/lib/i18n-source-check'
import { STABLE_I18N_CODES } from '../src/lib/message-codes'

const expectedLocales = ['zh-CN'] as const
const scriptDirectory = dirname(fileURLToPath(import.meta.url))
const localeDirectory = resolve(scriptDirectory, '../src/i18n/locales')
const sourceDirectory = resolve(scriptDirectory, '../src')
const bootstrapHtmlPath = resolve(scriptDirectory, '../index.html')
const dynamicRegistryPath = resolve(
  scriptDirectory,
  '../src/i18n/dynamic-keys.ts'
)

type LocaleMessages = Record<string, string>

async function readLocale(locale: string): Promise<LocaleMessages> {
  const path = resolve(localeDirectory, `${locale}.json`)
  const content = await readFile(path, 'utf8')
  const duplicateKeys = findDuplicateJsonKeys(content)
  if (duplicateKeys.length > 0) {
    throw new TypeError(
      `${locale}.json contains duplicate keys: ${duplicateKeys.join(', ')}`
    )
  }
  const parsed: unknown = JSON.parse(content)
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new TypeError(`${locale}.json must contain a JSON object`)
  }

  const messages: LocaleMessages = {}
  for (const [key, value] of Object.entries(parsed)) {
    if (typeof value !== 'string') {
      throw new TypeError(`${locale}.json key "${key}" must be a string`)
    }
    if (key.trim() === '') {
      throw new TypeError(`${locale}.json contains an empty key`)
    }
    if (value.trim() === '') {
      throw new TypeError(`${locale}.json key "${key}" has an empty value`)
    }
    messages[key] = value
  }
  return messages
}

async function sourceFiles(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true })
  const files: string[] = []
  for (const entry of entries) {
    const path = resolve(directory, entry.name)
    if (entry.isDirectory()) files.push(...(await sourceFiles(path)))
    else if (
      /\.tsx?$/.test(entry.name) &&
      !/\.test\.tsx?$/.test(entry.name) &&
      !entry.name.endsWith('.gen.ts')
    ) {
      files.push(path)
    }
  }
  return files
}

const localeFiles = (await readdir(localeDirectory))
  .filter((file) => file.endsWith('.json'))
  .map((file) => file.slice(0, -'.json'.length))
  .sort()
const expectedFiles = [...expectedLocales].sort()

if (localeFiles.join('\n') !== expectedFiles.join('\n')) {
  throw new Error(
    `Locale files must be exactly: ${expectedFiles.join(', ')}; found: ${localeFiles.join(', ')}`
  )
}

const locales = new Map<string, LocaleMessages>()
for (const locale of expectedLocales) {
  locales.set(locale, await readLocale(locale))
}

const baseline = locales.get('zh-CN')
if (!baseline) throw new Error('Simplified Chinese locale is required')
const baselineKeys = Object.keys(baseline).sort()

for (const code of STABLE_I18N_CODES) {
  if (!(code in baseline)) {
    throw new Error(
      `Simplified Chinese locale is missing stable code "${code}"`
    )
  }
}

const registryScan = parseDynamicI18nRegistries(
  await readFile(dynamicRegistryPath, 'utf8'),
  dynamicRegistryPath
)
if (registryScan.violations.length > 0) {
  throw new Error(
    `Dynamic i18n registries must be statically resolvable:\n${registryScan.violations.join('\n')}`
  )
}
for (const [registry, keys] of Object.entries(registryScan.registries)) {
  for (const key of keys) {
    if (!(key in baseline)) {
      throw new Error(
        `Dynamic i18n registry "${registry}" references missing key "${key}"`
      )
    }
  }
}

for (const [locale, messages] of locales) {
  const keys = Object.keys(messages).sort()
  const missing = baselineKeys.filter((key) => !(key in messages))
  const extra = keys.filter((key) => !(key in baseline))
  if (missing.length > 0 || extra.length > 0) {
    const details = [
      missing.length > 0 ? `missing: ${missing.join(', ')}` : '',
      extra.length > 0 ? `extra: ${extra.join(', ')}` : '',
    ]
      .filter(Boolean)
      .join('; ')
    throw new Error(
      `${locale}.json key set differs from zh-CN.json (${details})`
    )
  }
}

const hardcodedViolations: string[] = []
const dynamicKeyViolations: string[] = []
const dynamicRegistryUsages: DynamicRegistryUsage[] = []
const staticKeyUsages: TranslationKeyUsage[] = []
for (const path of await sourceFiles(sourceDirectory)) {
  const content = await readFile(path, 'utf8')
  for (const violation of findHardcodedVisibleText(content, path)) {
    hardcodedViolations.push(
      `${relative(sourceDirectory, path)}:${violation.line}:${violation.column} "${violation.value}"`
    )
  }
  const translationKeys = findTranslationKeyUsage(content, path)
  staticKeyUsages.push(...translationKeys.staticKeys)
  dynamicRegistryUsages.push(...translationKeys.dynamicRegistries)
  for (const violation of translationKeys.dynamicViolations) {
    dynamicKeyViolations.push(
      `${relative(sourceDirectory, path)}:${violation.line}:${violation.column} ${violation.value}`
    )
  }
}
if (hardcodedViolations.length > 0) {
  throw new Error(
    `User-visible text must use i18next:\n${hardcodedViolations.join('\n')}`
  )
}

const unknownRegistries = findUnknownDynamicRegistries(
  dynamicRegistryUsages,
  new Set(Object.keys(registryScan.registries))
)
if (unknownRegistries.length > 0) {
  throw new Error(
    `Translation calls use unknown dynamic registries: ${unknownRegistries.join(', ')}`
  )
}

if (dynamicKeyViolations.length > 0) {
  throw new Error(
    `Dynamic i18n keys must use dynamicI18nKey() and its registry:\n${dynamicKeyViolations.join('\n')}`
  )
}

const missingStaticKeys = findMissingTranslationKeys(
  staticKeyUsages,
  new Set(baselineKeys)
)
if (missingStaticKeys.length > 0) {
  throw new Error(
    `Simplified Chinese locale is missing referenced keys: ${missingStaticKeys.join(', ')}`
  )
}

const bootstrapViolations = findBootstrapHtmlViolations(
  await readFile(bootstrapHtmlPath, 'utf8')
)
if (bootstrapViolations.length > 0) {
  throw new Error(
    `index.html must leave runtime metadata to i18next:\n${bootstrapViolations.join('\n')}`
  )
}

process.stdout.write(
  `i18n check passed: ${expectedLocales.length} locale, ${baselineKeys.length} keys\n`
)
