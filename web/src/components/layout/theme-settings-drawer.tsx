import { PaintBoardIcon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { useSidebar } from '@/components/ui/sidebar'
import {
  type Collapsible,
  type Variant,
  useLayout,
} from '@/context/layout-provider'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { useTheme } from '@/context/theme-provider'
import {
  THEME_PRESETS,
  type ContentLayout,
  type ThemeFont,
  type ThemePreset,
  type ThemeRadius,
  type ThemeScale,
} from '@/lib/theme-customization'
import { cn } from '@/lib/utils'

function ChoiceGrid<TValue extends string>({
  columns = 3,
  onChange,
  options,
  value,
}: {
  columns?: 2 | 3 | 4 | 6
  onChange: (value: TValue) => void
  options: ReadonlyArray<{
    label: string
    preview?: React.ReactNode
    value: TValue
  }>
  value: TValue
}) {
  return (
    <div
      className={cn(
        'grid gap-3',
        columns === 2 && 'grid-cols-2',
        columns === 3 && 'grid-cols-3',
        columns === 4 && 'grid-cols-4',
        columns === 6 && 'grid-cols-3 sm:grid-cols-6'
      )}
    >
      {options.map((option) => {
        const selected = option.value === value
        return (
          <button
            aria-pressed={selected}
            className='group min-w-0 outline-none'
            key={option.value}
            onClick={() => onChange(option.value)}
            type='button'
          >
            <span
              className={cn(
                'ring-border group-focus-visible:ring-ring relative flex h-12 items-center justify-center overflow-hidden rounded-md ring-1 transition group-hover:ring-primary/60 group-focus-visible:ring-2',
                selected && 'ring-primary shadow-md ring-2'
              )}
            >
              {option.preview ?? (
                <span className='bg-foreground/60 h-1.5 w-3/4 rounded-full' />
              )}
              {selected && (
                <span className='bg-primary text-primary-foreground absolute top-1 right-1 flex size-5 items-center justify-center rounded-full'>
                  <HugeiconsIcon
                    icon={Tick02Icon}
                    size={13}
                    strokeWidth={2.5}
                  />
                </span>
              )}
            </span>
            <span className='mt-1.5 block truncate text-center text-xs'>
              {option.label}
            </span>
          </button>
        )
      })}
    </div>
  )
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h2 className='text-muted-foreground mb-2 text-sm font-semibold'>
      {children}
    </h2>
  )
}

export function ThemeSettingsDrawer() {
  const { t } = useTranslation()
  const { defaultTheme, resetTheme, setTheme, theme } = useTheme()
  const { open, setOpen } = useSidebar()
  const {
    collapsible,
    defaultCollapsible,
    defaultVariant,
    resetLayout,
    setCollapsible,
    setVariant,
    variant,
  } = useLayout()
  const {
    customization,
    resetCustomization,
    setContentLayout,
    setFont,
    setPreset,
    setRadius,
    setScale,
  } = useThemeCustomization()

  const presetLabels: Record<ThemePreset, string> = {
    default: t('preset.default'),
    anthropic: t('preset.anthropic'),
    'simple-large': t('preset.simple-large'),
    underground: t('preset.underground'),
    'rose-garden': t('preset.rose-garden'),
    'lake-view': t('preset.lake-view'),
    'sunset-glow': t('preset.sunset-glow'),
    'forest-whisper': t('preset.forest-whisper'),
    'ocean-breeze': t('preset.ocean-breeze'),
    'lavender-dream': t('preset.lavender-dream'),
  }

  const reset = () => {
    setOpen(true)
    resetTheme()
    resetLayout()
    resetCustomization()
  }

  const layoutMode = open ? 'default' : collapsible

  return (
    <Sheet>
      <SheetTrigger
        aria-label={t('Open theme settings')}
        render={
          <Button className='max-md:hidden' size='icon' variant='ghost' />
        }
        title={t('Open theme settings')}
      >
        <HugeiconsIcon icon={PaintBoardIcon} strokeWidth={2} />
      </SheetTrigger>
      <SheetContent className='bg-background text-foreground flex h-dvh w-full flex-col gap-0 overflow-hidden p-0 shadow-none sm:max-w-md'>
        <SheetHeader className='border-border/70 bg-background/95 supports-[backdrop-filter]:bg-background/80 border-b px-4 py-3 text-start backdrop-blur sm:px-6 sm:py-4'>
          <SheetTitle>{t('Theme Settings')}</SheetTitle>
          <SheetDescription>{t('Theme settings description')}</SheetDescription>
        </SheetHeader>
        <div className='flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto overscroll-contain px-4 py-4 sm:px-6 sm:py-5'>
          <section>
            <SectionTitle>{t('Theme')}</SectionTitle>
            <ChoiceGrid
              onChange={setTheme}
              options={[
                { label: t('System'), value: 'system' },
                { label: t('Light'), value: 'light' },
                { label: t('Dark'), value: 'dark' },
              ]}
              value={theme}
            />
          </section>

          <section>
            <SectionTitle>{t('Color preset')}</SectionTitle>
            <ChoiceGrid
              columns={4}
              onChange={setPreset}
              options={THEME_PRESETS.map((preset) => ({
                label: presetLabels[preset.value],
                preview: (
                  <span
                    className='absolute inset-0'
                    style={{
                      background:
                        preset.value === 'default'
                          ? 'linear-gradient(135deg, oklch(0.68 0.2 25), oklch(0.8 0.17 85), oklch(0.72 0.18 155), oklch(0.66 0.19 245), oklch(0.68 0.2 315))'
                          : `linear-gradient(135deg, ${preset.swatches[0]}, ${preset.swatches[1]})`,
                    }}
                  />
                ),
                value: preset.value,
              }))}
              value={customization.preset}
            />
          </section>

          <section>
            <SectionTitle>{t('Font')}</SectionTitle>
            <ChoiceGrid<ThemeFont>
              onChange={setFont}
              options={[
                { label: t('Auto'), value: 'default' },
                {
                  label: t('Sans'),
                  // i18n-ignore: universal font specimen
                  preview: <span className='font-sans text-lg'>Aa</span>,
                  value: 'sans',
                },
                {
                  label: t('Serif'),
                  preview: (
                    // i18n-ignore: universal font specimen
                    <span className='text-lg font-[var(--font-serif)]'>Aa</span>
                  ),
                  value: 'serif',
                },
              ]}
              value={customization.font}
            />
          </section>

          <section>
            <SectionTitle>{t('Border radius')}</SectionTitle>
            <ChoiceGrid<ThemeRadius>
              columns={6}
              onChange={setRadius}
              options={[
                { label: t('Auto'), value: 'default' },
                { label: '0', value: 'none' },
                { label: '0.3', value: 'sm' },
                { label: '0.5', value: 'md' },
                { label: '0.75', value: 'lg' },
                { label: '1.0', value: 'xl' },
              ]}
              value={customization.radius}
            />
          </section>

          <section>
            <SectionTitle>{t('Density')}</SectionTitle>
            <ChoiceGrid<ThemeScale>
              columns={4}
              onChange={setScale}
              options={[
                { label: t('Compact'), value: 'sm' },
                { label: t('Default'), value: 'default' },
                { label: t('Comfortable'), value: 'lg' },
                { label: t('Super Large'), value: 'xl' },
              ]}
              value={customization.scale}
            />
          </section>

          <section className='max-md:hidden'>
            <SectionTitle>{t('Sidebar')}</SectionTitle>
            <ChoiceGrid<Variant>
              onChange={setVariant}
              options={[
                { label: t('Inset'), value: 'inset' },
                { label: t('Floating'), value: 'floating' },
                { label: t('Sidebar'), value: 'sidebar' },
              ]}
              value={variant}
            />
          </section>

          <section className='max-md:hidden'>
            <SectionTitle>{t('Layout')}</SectionTitle>
            <ChoiceGrid<'default' | Collapsible>
              onChange={(value) => {
                if (value === 'default') {
                  setOpen(true)
                  return
                }
                setCollapsible(value)
                setOpen(false)
              }}
              options={[
                { label: t('Default'), value: 'default' },
                { label: t('Compact'), value: 'icon' },
                { label: t('Full layout'), value: 'offcanvas' },
              ]}
              value={layoutMode}
            />
          </section>

          <section>
            <SectionTitle>{t('Content width')}</SectionTitle>
            <ChoiceGrid<ContentLayout>
              columns={2}
              onChange={setContentLayout}
              options={[
                { label: t('Full width'), value: 'full' },
                { label: t('Centered'), value: 'centered' },
              ]}
              value={customization.contentLayout}
            />
          </section>
        </div>
        <SheetFooter className='border-border/70 bg-background/95 supports-[backdrop-filter]:bg-background/80 grid grid-cols-1 gap-2 border-t px-4 py-3 backdrop-blur sm:flex sm:flex-row sm:justify-end sm:px-6 sm:py-4'>
          <Button
            className='w-full'
            onClick={reset}
            variant={
              theme === defaultTheme &&
              customization.preset === 'default' &&
              customization.font === 'default' &&
              customization.radius === 'default' &&
              customization.scale === 'default' &&
              customization.contentLayout === 'full' &&
              variant === defaultVariant &&
              collapsible === defaultCollapsible &&
              open
                ? 'secondary'
                : 'destructive'
            }
          >
            {t('Reset')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
