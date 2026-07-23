import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { SettingsPage } from '@/features/settings/components/settings-page'
import {
  settingsSectionKeys,
  type SettingsSectionKey,
} from '@/features/settings/contract'

const settingsSearchSchema = z.object({
  section: z.enum(settingsSectionKeys).optional().catch(undefined),
})

export const Route = createFileRoute('/_authenticated/settings/system')({
  component: SystemSettingsRoute,
  validateSearch: settingsSearchSchema,
})

function SystemSettingsRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const activeSection = search.section ?? 'collection'

  return (
    <SettingsPage
      activeSection={activeSection}
      onSectionChange={(section: SettingsSectionKey) => {
        void navigate({
          replace: true,
          search: section === 'collection' ? {} : { section },
        })
      }}
    />
  )
}
