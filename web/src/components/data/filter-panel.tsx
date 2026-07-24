import { ArrowDown01Icon, Cancel01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  Children,
  cloneElement,
  isValidElement,
  useEffect,
  useState,
  type ReactElement,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SelectControl as Select } from '@/components/ui/select-control'
import { cn } from '@/lib/utils'

type FilterElementProps = {
  'aria-label'?: string
  children?: ReactNode
  className?: string
  placeholder?: string
  value?: string
}

function textContent(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(textContent).join('')
  if (isValidElement(node)) {
    return textContent(
      (node as ReactElement<FilterElementProps>).props.children
    )
  }
  return ''
}

function compactFilterFields(children: ReactNode): ReactNode {
  const compacted = Children.toArray(children).map((child) => {
    if (!isValidElement(child)) return child

    if (child.type === 'label') {
      const label = child as ReactElement<FilterElementProps>
      const labelChildren = Children.toArray(label.props.children)
      const labelNode = labelChildren.find(
        (item) => isValidElement(item) && item.type === 'span'
      )
      const fieldName = labelNode ? textContent(labelNode) : ''
      const nextChildren = labelChildren.map((item) => {
        if (item === labelNode && isValidElement(item)) {
          return cloneElement(item as ReactElement<FilterElementProps>, {
            className: 'sr-only',
          })
        }
        if (!isValidElement(item)) return item

        const element = item as ReactElement<FilterElementProps>

        if (
          element.type === Input ||
          element.type === 'input' ||
          element.type === 'textarea'
        ) {
          return cloneElement(element, {
            'aria-label': element.props['aria-label'] ?? fieldName,
            placeholder: element.props.placeholder || fieldName,
          })
        }

        if (element.type === Select || element.type === 'select') {
          const options = Children.map(element.props.children, (option) => {
            if (
              isValidElement(option) &&
              option.type === 'option' &&
              (option as ReactElement<FilterElementProps>).props.value === ''
            ) {
              return cloneElement(option as ReactElement<FilterElementProps>, {
                children: fieldName,
              })
            }
            return option
          })
          return cloneElement(element, {
            'aria-label': element.props['aria-label'] ?? fieldName,
            children: options,
          })
        }

        return item
      })
      return cloneElement(label, { children: nextChildren })
    }

    const element = child as ReactElement<FilterElementProps>
    if (element.props.children == null) return child
    return cloneElement(element, {
      children: compactFilterFields(element.props.children),
    })
  })
  return compacted.length === 1 ? compacted[0] : compacted
}

export function FilterPanel({
  actions,
  advanced,
  children,
  description,
  expandOnLargeScreen = false,
  hasAdvancedActive = false,
  hasActiveFilters,
  onApply,
  onReset,
  title,
}: {
  actions?: React.ReactNode
  advanced?: React.ReactNode
  children: React.ReactNode
  description: string
  expandOnLargeScreen?: boolean
  hasAdvancedActive?: boolean
  hasActiveFilters?: boolean
  onApply?: () => void
  onReset?: () => void
  title: string
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(hasAdvancedActive)
  const [isLargeScreen, setIsLargeScreen] = useState(false)
  const hasAdvanced = advanced != null

  useEffect(() => {
    if (!expandOnLargeScreen) return
    const mediaQuery = window.matchMedia('(min-width: 1280px)')
    const update = () => setIsLargeScreen(mediaQuery.matches)
    update()
    mediaQuery.addEventListener('change', update)
    return () => mediaQuery.removeEventListener('change', update)
  }, [expandOnLargeScreen])

  const advancedVisible = expanded || (expandOnLargeScreen && isLargeScreen)

  return (
    <section aria-label={title} className='flex min-w-0 flex-col gap-2'>
      <span className='sr-only'>{description}</span>
      <div className='flex flex-wrap items-center gap-2 sm:gap-3'>
        <div className='flex w-full min-w-0 flex-1 flex-wrap items-center gap-2 sm:w-auto sm:gap-3'>
          {compactFilterFields(children)}
        </div>
        <div className='ms-auto flex shrink-0 items-center gap-1.5 sm:gap-2'>
          {onReset && (hasActiveFilters ?? true) && (
            <Button
              className={
                onApply
                  ? undefined
                  : 'text-muted-foreground hover:text-foreground gap-1 px-2'
              }
              onClick={onReset}
              type='button'
              variant={onApply ? 'outline' : 'ghost'}
            >
              {t('common.reset')}
              {!onApply && (
                <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
              )}
            </Button>
          )}
          {onApply && (
            <Button onClick={onApply} type='button'>
              {t('common.search')}
            </Button>
          )}
          {actions}
          {hasAdvanced && !(expandOnLargeScreen && isLargeScreen) && (
            <Button
              aria-expanded={expanded}
              className={cn(
                'text-muted-foreground hover:text-foreground shrink-0 gap-1 px-2',
                hasAdvancedActive &&
                  !expanded &&
                  'text-primary hover:text-primary'
              )}
              onClick={() => setExpanded((current) => !current)}
              type='button'
              variant='ghost'
            >
              {expanded ? t('common.collapse') : t('common.expand')}
              <HugeiconsIcon
                className={cn(
                  'size-3.5 transition-transform duration-200',
                  expanded && 'rotate-180'
                )}
                icon={ArrowDown01Icon}
                strokeWidth={2}
              />
            </Button>
          )}
        </div>
      </div>
      {advancedVisible && advanced && (
        <div className='flex flex-wrap items-center gap-2 sm:gap-3'>
          {compactFilterFields(advanced)}
        </div>
      )}
    </section>
  )
}
