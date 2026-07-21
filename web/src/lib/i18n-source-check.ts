import { parse as parseJavaScript, parseExpression } from '@babel/parser'
import { parse as parseHtml } from 'parse5'

interface AstNode {
  type: string
  loc?: { start: { column: number; line: number } } | null
  [key: string]: unknown
}

interface HtmlAttribute {
  name: string
  value: string
}

interface HtmlNode {
  attrs?: HtmlAttribute[]
  childNodes?: HtmlNode[]
  nodeName?: string
  tagName?: string
  value?: string
}

export interface SourceViolation {
  column: number
  line: number
  value: string
}

export interface TranslationKeyUsage extends SourceViolation {
  key: string
}

export interface DynamicRegistryUsage extends SourceViolation {
  registry: string
}

export interface TranslationKeyScan {
  dynamicRegistries: DynamicRegistryUsage[]
  dynamicViolations: SourceViolation[]
  staticKeys: TranslationKeyUsage[]
}

export interface DynamicRegistryScan {
  registries: Record<string, string[]>
  violations: string[]
}

function isAstNode(value: unknown): value is AstNode {
  return Boolean(
    value &&
    typeof value === 'object' &&
    typeof (value as { type?: unknown }).type === 'string'
  )
}

function objectPropertyKey(node: AstNode): string | null {
  const key = node.key
  if (!isAstNode(key)) return null
  if (key.type === 'StringLiteral' && typeof key.value === 'string') {
    return key.value
  }
  return null
}

export function findDuplicateJsonKeys(source: string): string[] {
  const expression = parseExpression(source)
  if (!isAstNode(expression) || expression.type !== 'ObjectExpression') {
    return []
  }
  const properties = Array.isArray(expression.properties)
    ? expression.properties
    : []
  const seen = new Set<string>()
  const duplicates = new Set<string>()
  for (const property of properties) {
    if (!isAstNode(property) || property.type !== 'ObjectProperty') continue
    const key = objectPropertyKey(property)
    if (key == null) continue
    if (seen.has(key)) duplicates.add(key)
    seen.add(key)
  }
  return [...duplicates].sort()
}

const visibleTextPattern = /[\p{L}\p{N}$]/u

export const VISIBLE_JSX_PROPS = new Set([
  'alt',
  'aria-description',
  'aria-label',
  'cancelLabel',
  'caption',
  'confirmLabel',
  'description',
  'emptyDescription',
  'emptyMessage',
  'emptyTitle',
  'errorMessage',
  'helpText',
  'label',
  'message',
  'placeholder',
  'retryLabel',
  'submitLabel',
  'successMessage',
  'title',
  'tooltip',
])

const ignoredNodeKeys = new Set([
  'comments',
  'errors',
  'extra',
  'loc',
  'start',
  'end',
  'tokens',
])

function literalValue(node: AstNode): string | null {
  if (
    (node.type === 'StringLiteral' || node.type === 'JSXText') &&
    typeof node.value === 'string'
  ) {
    return node.value
  }
  if (node.type === 'TemplateElement') {
    const value = node.value
    if (
      value &&
      typeof value === 'object' &&
      typeof (value as { raw?: unknown }).raw === 'string'
    ) {
      return (value as { raw: string }).raw
    }
  }
  return null
}

function nodeName(node: unknown): string | null {
  return isAstNode(node) &&
    (node.type === 'JSXIdentifier' || node.type === 'Identifier') &&
    typeof node.name === 'string'
    ? node.name
    : null
}

function memberName(node: unknown): string | null {
  if (!isAstNode(node) || node.type !== 'MemberExpression') return null
  return nodeName(node.property)
}

function isTranslationCall(node: AstNode): boolean {
  if (node.type !== 'CallExpression') return false
  const callee = node.callee
  if (nodeName(callee) === 't') return true
  if (!isAstNode(callee) || callee.type !== 'MemberExpression') return false
  return nodeName(callee.object) === 'i18n' && memberName(callee) === 't'
}

function staticTranslationKey(node: AstNode | undefined): string | null {
  if (!node) return null
  if (node.type === 'StringLiteral' && typeof node.value === 'string') {
    return node.value
  }
  if (
    node.type === 'TemplateLiteral' &&
    Array.isArray(node.expressions) &&
    node.expressions.length === 0 &&
    Array.isArray(node.quasis) &&
    node.quasis.length === 1 &&
    isAstNode(node.quasis[0])
  ) {
    return literalValue(node.quasis[0])
  }
  return null
}

function registeredDynamicTranslationRegistry(
  node: AstNode | undefined
): string | null {
  if (
    node?.type !== 'CallExpression' ||
    nodeName(node.callee) !== 'dynamicI18nKey' ||
    !Array.isArray(node.arguments) ||
    node.arguments.length < 2
  ) {
    return null
  }
  const registry = node.arguments[0]
  return isAstNode(registry) &&
    registry.type === 'StringLiteral' &&
    typeof registry.value === 'string'
    ? registry.value
    : null
}

export function findTranslationKeyUsage(
  source: string,
  filename: string
): TranslationKeyScan {
  const ast = parseJavaScript(source, {
    plugins: filename.endsWith('.tsx') ? ['typescript', 'jsx'] : ['typescript'],
    sourceType: 'module',
  })
  const staticKeys: TranslationKeyUsage[] = []
  const dynamicViolations: SourceViolation[] = []
  const dynamicRegistries: DynamicRegistryUsage[] = []

  const visit = (value: unknown) => {
    if (Array.isArray(value)) {
      for (const item of value) visit(item)
      return
    }
    if (!isAstNode(value)) return

    if (isTranslationCall(value)) {
      const argumentsValue = Array.isArray(value.arguments)
        ? value.arguments
        : []
      const firstArgument = isAstNode(argumentsValue[0])
        ? argumentsValue[0]
        : undefined
      const key = staticTranslationKey(firstArgument)
      const line = value.loc?.start.line ?? 1
      const column = (value.loc?.start.column ?? 0) + 1
      if (key != null) {
        staticKeys.push({ column, key, line, value: key })
      } else {
        const registry = registeredDynamicTranslationRegistry(firstArgument)
        if (registry) {
          dynamicRegistries.push({
            column,
            line,
            registry,
            value: registry,
          })
        } else {
          dynamicViolations.push({
            column,
            line,
            value: firstArgument?.type ?? 'missing argument',
          })
        }
      }
    }

    for (const [key, child] of Object.entries(value)) {
      if (!ignoredNodeKeys.has(key)) visit(child)
    }
  }

  visit(ast)
  return { dynamicRegistries, dynamicViolations, staticKeys }
}

export function findMissingTranslationKeys(
  usages: readonly TranslationKeyUsage[],
  availableKeys: ReadonlySet<string>
): string[] {
  return [
    ...new Set(
      usages
        .filter((usage) => !availableKeys.has(usage.key))
        .map((usage) => usage.key)
    ),
  ].sort()
}

function unwrapTypeExpression(node: AstNode): AstNode {
  let current = node
  while (
    [
      'TSAsExpression',
      'TSSatisfiesExpression',
      'TSNonNullExpression',
      'TypeCastExpression',
    ].includes(current.type) &&
    isAstNode(current.expression)
  ) {
    current = current.expression
  }
  return current
}

function propertyName(node: AstNode): string | null {
  const key = node.key
  if (!isAstNode(key)) return null
  if (
    (key.type === 'Identifier' || key.type === 'StringLiteral') &&
    typeof (key.name ?? key.value) === 'string'
  ) {
    return String(key.name ?? key.value)
  }
  return null
}

export function parseDynamicI18nRegistries(
  source: string,
  filename = 'dynamic-keys.ts'
): DynamicRegistryScan {
  const ast = parseJavaScript(source, {
    plugins: filename.endsWith('.tsx') ? ['typescript', 'jsx'] : ['typescript'],
    sourceType: 'module',
  })
  const registries: Record<string, string[]> = {}
  const violations: string[] = []
  let registryObject: AstNode | null = null

  const visit = (value: unknown) => {
    if (Array.isArray(value)) {
      for (const item of value) visit(item)
      return
    }
    if (!isAstNode(value)) return
    if (
      value.type === 'VariableDeclarator' &&
      nodeName(value.id) === 'DYNAMIC_I18N_REGISTRIES'
    ) {
      if (isAstNode(value.init)) {
        registryObject = unwrapTypeExpression(value.init)
      }
    }
    for (const [key, child] of Object.entries(value)) {
      if (!ignoredNodeKeys.has(key)) visit(child)
    }
  }
  visit(ast)
  const parsedRegistryObject = registryObject as AstNode | null
  if (
    !parsedRegistryObject ||
    parsedRegistryObject.type !== 'ObjectExpression'
  ) {
    return {
      registries,
      violations: ['DYNAMIC_I18N_REGISTRIES must be a static object literal'],
    }
  }
  const properties = Array.isArray(parsedRegistryObject.properties)
    ? parsedRegistryObject.properties
    : []
  for (const property of properties) {
    if (!isAstNode(property) || property.type !== 'ObjectProperty') {
      violations.push('registry entries must be plain object properties')
      continue
    }
    const name = propertyName(property)
    if (!name || !isAstNode(property.value)) {
      violations.push('registry names must be static identifiers or strings')
      continue
    }
    const array = unwrapTypeExpression(property.value)
    if (array.type !== 'ArrayExpression' || !Array.isArray(array.elements)) {
      violations.push(`registry "${name}" must be a static string array`)
      continue
    }
    const keys: string[] = []
    for (const element of array.elements) {
      if (
        !isAstNode(element) ||
        element.type !== 'StringLiteral' ||
        typeof element.value !== 'string'
      ) {
        violations.push(`registry "${name}" contains a non-string entry`)
        continue
      }
      keys.push(element.value)
    }
    if (new Set(keys).size !== keys.length) {
      violations.push(`registry "${name}" contains duplicate keys`)
    }
    registries[name] = keys
  }
  return { registries, violations }
}

export function findUnknownDynamicRegistries(
  usages: readonly DynamicRegistryUsage[],
  registryNames: ReadonlySet<string>
): string[] {
  return [
    ...new Set(
      usages
        .filter((usage) => !registryNames.has(usage.registry))
        .map((usage) => usage.registry)
    ),
  ].sort()
}

function isInsideTranslationCall(ancestors: readonly AstNode[]): boolean {
  return ancestors.some(isTranslationCall)
}

function includesNode(value: unknown, node: AstNode): boolean {
  return Array.isArray(value) && value.includes(node)
}

function isTransparentRenderedParent(parent: AstNode, child: AstNode): boolean {
  if (parent.type === 'ConditionalExpression') {
    return parent.consequent === child || parent.alternate === child
  }
  if (parent.type === 'LogicalExpression') {
    return parent.left === child || parent.right === child
  }
  if (parent.type === 'TemplateLiteral') {
    return (
      includesNode(parent.expressions, child) ||
      includesNode(parent.quasis, child)
    )
  }
  if (parent.type === 'BinaryExpression') {
    return (
      parent.operator === '+' &&
      (parent.left === child || parent.right === child)
    )
  }
  if (
    [
      'ParenthesizedExpression',
      'TSAsExpression',
      'TSNonNullExpression',
      'TSTypeAssertion',
      'TypeCastExpression',
    ].includes(parent.type)
  ) {
    return parent.expression === child
  }
  return false
}

function hasTransparentRenderedPath(
  node: AstNode,
  ancestors: readonly AstNode[],
  boundaryIndex: number
): boolean {
  let child = node
  for (let index = ancestors.length - 1; index > boundaryIndex; index -= 1) {
    const parent = ancestors[index]
    if (!isTransparentRenderedParent(parent, child)) return false
    child = parent
  }
  return true
}

function jsxLiteralVisibility(
  node: AstNode,
  ancestors: readonly AstNode[]
): boolean | null {
  let expressionIndex = -1
  let attributeIndex = -1
  for (let index = 0; index < ancestors.length; index += 1) {
    if (ancestors[index].type === 'JSXExpressionContainer') {
      expressionIndex = index
    }
    if (ancestors[index].type === 'JSXAttribute') attributeIndex = index
  }

  if (attributeIndex > expressionIndex) {
    return VISIBLE_JSX_PROPS.has(nodeName(ancestors[attributeIndex].name) ?? '')
  }
  if (expressionIndex >= 0) {
    if (!hasTransparentRenderedPath(node, ancestors, expressionIndex)) {
      return false
    }
    if (attributeIndex === expressionIndex - 1) {
      return VISIBLE_JSX_PROPS.has(
        nodeName(ancestors[attributeIndex].name) ?? ''
      )
    }
    return true
  }
  return null
}

function directChildWithinAncestor(
  ancestorIndex: number,
  node: AstNode,
  ancestors: readonly AstNode[]
): AstNode {
  return ancestors[ancestorIndex + 1] ?? node
}

function isInsideVisibleCall(
  node: AstNode,
  ancestors: readonly AstNode[]
): boolean {
  return ancestors.some((ancestor, index) => {
    if (ancestor.type !== 'CallExpression') return false
    const directName = nodeName(ancestor.callee)
    const method = memberName(ancestor.callee)
    const isVisibleSink =
      ['alert', 'confirm', 'prompt'].includes(directName ?? '') ||
      ['error', 'info', 'success', 'warning'].includes(method ?? '')
    if (!isVisibleSink || !Array.isArray(ancestor.arguments)) return false
    return (
      ancestor.arguments[0] ===
        directChildWithinAncestor(index, node, ancestors) &&
      hasTransparentRenderedPath(node, ancestors, index)
    )
  })
}

function isInsideDocumentTitleAssignment(
  node: AstNode,
  ancestors: readonly AstNode[]
): boolean {
  return ancestors.some((ancestor, index) => {
    if (
      ancestor.type !== 'AssignmentExpression' ||
      memberName(ancestor.left) !== 'title'
    ) {
      return false
    }
    return (
      ancestor.right === directChildWithinAncestor(index, node, ancestors) &&
      hasTransparentRenderedPath(node, ancestors, index)
    )
  })
}

function isVisibleLiteralContext(
  node: AstNode,
  ancestors: readonly AstNode[]
): boolean {
  if (node.type === 'JSXText') return true
  if (isInsideTranslationCall(ancestors)) return false
  const jsxVisibility = jsxLiteralVisibility(node, ancestors)
  if (jsxVisibility != null) return jsxVisibility
  return (
    isInsideVisibleCall(node, ancestors) ||
    isInsideDocumentTitleAssignment(node, ancestors)
  )
}

export function findHardcodedVisibleText(
  source: string,
  filename: string
): SourceViolation[] {
  const ast = parseJavaScript(source, {
    plugins: filename.endsWith('.tsx') ? ['typescript', 'jsx'] : ['typescript'],
    sourceType: 'module',
  })
  const lines = source.split(/\r?\n/)
  const violations: SourceViolation[] = []

  const visit = (value: unknown, ancestors: readonly AstNode[] = []) => {
    if (Array.isArray(value)) {
      for (const item of value) visit(item, ancestors)
      return
    }
    if (!isAstNode(value)) return

    const literal = literalValue(value)
    if (
      literal &&
      visibleTextPattern.test(literal) &&
      isVisibleLiteralContext(value, ancestors)
    ) {
      const line = value.loc?.start.line ?? 1
      const currentLine = lines[line - 1] ?? ''
      const previousLine = lines[line - 2] ?? ''
      if (
        !currentLine.includes('i18n-ignore') &&
        !previousLine.includes('i18n-ignore')
      ) {
        violations.push({
          column: (value.loc?.start.column ?? 0) + 1,
          line,
          value: literal.trim().replaceAll(/\s+/g, ' ').slice(0, 80),
        })
      }
    }

    for (const [key, child] of Object.entries(value)) {
      if (!ignoredNodeKeys.has(key)) visit(child, [...ancestors, value])
    }
  }

  visit(ast)
  return violations
}

function htmlTextContent(node: HtmlNode): string {
  if (node.nodeName === '#text') return node.value ?? ''
  return (node.childNodes ?? []).map(htmlTextContent).join('')
}

function htmlElements(node: HtmlNode): HtmlNode[] {
  const descendants = (node.childNodes ?? []).flatMap(htmlElements)
  return node.tagName ? [node, ...descendants] : descendants
}

function htmlAttribute(node: HtmlNode, name: string): string | null {
  return node.attrs?.find((attribute) => attribute.name === name)?.value ?? null
}

export function findBootstrapHtmlViolations(source: string): string[] {
  const document = parseHtml(source) as unknown as HtmlNode
  const elements = htmlElements(document)
  const titles = elements.filter((element) => element.tagName === 'title')
  const descriptions = elements.filter(
    (element) =>
      element.tagName === 'meta' &&
      htmlAttribute(element, 'name')?.toLowerCase() === 'description'
  )
  const violations: string[] = []

  if (titles.length !== 1) {
    violations.push(`expected exactly one <title>; found ${titles.length}`)
  } else if (htmlTextContent(titles[0]).trim() !== '') {
    violations.push('<title> must be empty; i18next sets it at runtime')
  }

  if (descriptions.length !== 1) {
    violations.push(
      `expected exactly one description meta; found ${descriptions.length}`
    )
  } else {
    const content = htmlAttribute(descriptions[0], 'content')
    if (content == null || content.trim() !== '') {
      violations.push(
        'description meta content must be present and empty; i18next sets it at runtime'
      )
    }
  }

  return violations
}
