import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ComponentProps,
} from 'react'

import { Input } from '@/components/ui/input'

type DebouncedInputProps = Omit<
  ComponentProps<typeof Input>,
  'onBlur' | 'onChange' | 'value'
> & {
  delay?: number
  onValueChange: (value: string) => void
  value: string
}

export function DebouncedInput({
  delay = 400,
  onValueChange,
  value,
  ...props
}: DebouncedInputProps) {
  const [draft, setDraft] = useState(value)
  const generationRef = useRef(0)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const cancelPending = useCallback(() => {
    generationRef.current += 1
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = null
  }, [])

  useEffect(() => {
    cancelPending()
    setDraft(value)
  }, [cancelPending, value])
  useEffect(() => () => cancelPending(), [cancelPending])

  const commit = (next: string) => {
    cancelPending()
    if (next !== value) onValueChange(next)
  }

  return (
    <Input
      {...props}
      onBlur={() => commit(draft)}
      onChange={(event) => {
        const next = event.target.value
        setDraft(next)
        if (timerRef.current) clearTimeout(timerRef.current)
        const generation = ++generationRef.current
        timerRef.current = setTimeout(() => {
          if (generation !== generationRef.current) return
          timerRef.current = null
          if (next !== value) onValueChange(next)
        }, delay)
      }}
      value={draft}
    />
  )
}
