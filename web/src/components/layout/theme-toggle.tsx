import { Moon02Icon, Sun01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { useTheme } from '@/context/theme-provider'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import { Button } from '../ui/button'

export function ThemeToggle() {
  const { t } = useTranslation()
  const { resolvedTheme, setPreference } = useTheme()
  const nextTheme = resolvedTheme === 'dark' ? 'light' : 'dark'
  const label =
    nextTheme === 'dark' ? 'Switch to dark theme' : 'Switch to light theme'

  return (
    <Button
      aria-label={t(dynamicI18nKey('layout', label))}
      onClick={() => setPreference(nextTheme)}
      size='icon'
      title={t(dynamicI18nKey('layout', label))}
      variant='ghost'
    >
      <HugeiconsIcon
        icon={resolvedTheme === 'dark' ? Sun01Icon : Moon02Icon}
        strokeWidth={2}
      />
    </Button>
  )
}
