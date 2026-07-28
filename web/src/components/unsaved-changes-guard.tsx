import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/ui/confirm-dialog'

export function UnsavedChangesConfirmDialog({
  onConfirm,
  onOpenChange,
  open,
}: {
  onConfirm: () => void
  onOpenChange: (open: boolean) => void
  open: boolean
}) {
  const { t } = useTranslation()
  return (
    <ConfirmDialog
      confirmLabel={t('common.discardChanges')}
      description={t('common.unsavedChangesDescription')}
      onConfirm={onConfirm}
      onOpenChange={onOpenChange}
      open={open}
      title={t('common.unsavedChangesTitle')}
    />
  )
}
