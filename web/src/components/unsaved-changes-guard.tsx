import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/ui/confirm-dialog'

export function UnsavedChangesConfirmDialog({
  onConfirm,
  onOpenChange,
  open,
  pending = false,
}: {
  onConfirm: () => void
  onOpenChange: (open: boolean) => void
  open: boolean
  pending?: boolean
}) {
  const { t } = useTranslation()
  return (
    <ConfirmDialog
      confirmLabel={t('common.discardChanges')}
      description={t('common.unsavedChangesDescription')}
      onConfirm={onConfirm}
      onOpenChange={onOpenChange}
      open={open}
      pending={pending}
      title={t('common.unsavedChangesTitle')}
    />
  )
}
