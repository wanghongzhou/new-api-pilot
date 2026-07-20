import { EyeIcon, EyeOffIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { dynamicI18nKey } from '@/i18n/dynamic-keys'
import { cn } from '@/lib/utils'

import { Button } from './button'
import { Input } from './input'

export function PasswordInput({
  className,
  ...props
}: Omit<ComponentProps<typeof Input>, 'type'>) {
  const { t } = useTranslation()
  const [visible, setVisible] = useState(false)
  const label = visible ? 'Hide password' : 'Show password'

  return (
    <div className={cn('relative', className)}>
      <Input
        className='pr-11'
        type={visible ? 'text' : 'password'}
        {...props}
      />
      <Button
        aria-label={t(dynamicI18nKey('layout', label))}
        className='absolute top-0 right-0'
        onClick={() => setVisible((current) => !current)}
        size='icon'
        title={t(dynamicI18nKey('layout', label))}
        type='button'
        variant='ghost'
      >
        <HugeiconsIcon icon={visible ? EyeOffIcon : EyeIcon} strokeWidth={2} />
      </Button>
    </div>
  )
}
