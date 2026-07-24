import {
  Children,
  Fragment,
  isValidElement,
  useState,
  type ChangeEvent,
  type ReactElement,
  type ReactNode,
} from 'react'

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'

const EMPTY_VALUE = '__pilot_select_empty__'

type OptionProps = React.ComponentProps<'option'>
type OptGroupProps = React.ComponentProps<'optgroup'>

export type SelectControlProps = {
  'aria-describedby'?: string
  'aria-invalid'?: boolean | 'false' | 'true'
  'aria-label'?: string
  'aria-labelledby'?: string
  alignItemWithTrigger?: boolean
  children: ReactNode
  className?: string
  defaultValue?: number | string
  disabled?: boolean
  id?: string
  name?: string
  onChange?: (event: ChangeEvent<HTMLSelectElement>) => void
  portalled?: boolean
  required?: boolean
  size?: 'default' | 'sm'
  value?: number | string
}

function internalValue(
  value: number | readonly string[] | string | null | undefined
) {
  const normalized = value == null ? '' : String(value)
  return normalized === '' ? EMPTY_VALUE : normalized
}

function externalValue(value: unknown) {
  if (value == null || value === EMPTY_VALUE) return ''
  return String(value)
}

function collectOptions(children: ReactNode): ReactElement<OptionProps>[] {
  const options: ReactElement<OptionProps>[] = []
  Children.forEach(children, (child) => {
    if (!isValidElement(child)) return
    if (child.type === Fragment) {
      options.push(
        ...collectOptions(
          (child as ReactElement<{ children?: ReactNode }>).props.children
        )
      )
      return
    }
    if (child.type === 'option') {
      options.push(child as ReactElement<OptionProps>)
      return
    }
    if (child.type === 'optgroup') {
      options.push(
        ...collectOptions((child as ReactElement<OptGroupProps>).props.children)
      )
    }
  })
  return options
}

function renderOptions(children: ReactNode): ReactNode {
  return Children.map(children, (child) => {
    if (!isValidElement(child)) return null
    if (child.type === Fragment) {
      return renderOptions(
        (child as ReactElement<{ children?: ReactNode }>).props.children
      )
    }
    if (child.type === 'option') {
      const option = child as ReactElement<OptionProps>
      return (
        <SelectItem
          data-select-value={externalValue(option.props.value)}
          disabled={option.props.disabled}
          key={option.key ?? String(option.props.value)}
          value={internalValue(option.props.value)}
        >
          {option.props.children}
        </SelectItem>
      )
    }
    if (child.type === 'optgroup') {
      const group = child as ReactElement<OptGroupProps>
      return (
        <SelectGroup key={group.key ?? String(group.props.label)}>
          {group.props.label && <SelectLabel>{group.props.label}</SelectLabel>}
          {renderOptions(group.props.children)}
        </SelectGroup>
      )
    }
    return null
  })
}

export function SelectControl({
  'aria-describedby': ariaDescribedBy,
  'aria-invalid': ariaInvalid,
  'aria-label': ariaLabel,
  'aria-labelledby': ariaLabelledBy,
  alignItemWithTrigger = false,
  children,
  className,
  defaultValue,
  disabled,
  id,
  name,
  onChange,
  portalled,
  required,
  size,
  value,
}: SelectControlProps) {
  const options = collectOptions(children)
  const firstValue = options.find((option) => !option.props.disabled)?.props
    .value
  const controlled = value != null
  const [open, setOpen] = useState(false)
  const [uncontrolledValue, setUncontrolledValue] = useState(() =>
    internalValue(defaultValue ?? firstValue)
  )
  const selectedValue = controlled ? internalValue(value) : uncontrolledValue
  const selectedOption = options.find(
    (option) => internalValue(option.props.value) === selectedValue
  )

  return (
    <Select
      disabled={disabled}
      name={name}
      onOpenChange={setOpen}
      onValueChange={(nextValue) => {
        const next = externalValue(nextValue)
        setOpen(false)
        if (!controlled) setUncontrolledValue(internalValue(next))
        if (onChange) {
          const target = { name: name ?? '', value: next } as HTMLSelectElement
          onChange({
            currentTarget: target,
            target,
          } as ChangeEvent<HTMLSelectElement>)
        }
      }}
      open={open}
      value={selectedValue}
    >
      <SelectTrigger
        aria-describedby={ariaDescribedBy}
        aria-invalid={ariaInvalid}
        aria-label={ariaLabel}
        aria-labelledby={ariaLabelledBy}
        aria-required={required}
        className={cn('w-full', className)}
        id={id}
        size={size}
      >
        <SelectValue>{selectedOption?.props.children}</SelectValue>
      </SelectTrigger>
      <SelectContent
        alignItemWithTrigger={alignItemWithTrigger}
        onClick={(event) => {
          event.preventDefault()
          event.stopPropagation()
        }}
        portalled={portalled}
      >
        {renderOptions(children)}
      </SelectContent>
    </Select>
  )
}
