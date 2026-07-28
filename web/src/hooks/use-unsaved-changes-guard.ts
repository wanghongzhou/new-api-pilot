import { useCallback, useState } from 'react'

export function shouldConfirmDiscard(hasUnsavedChanges: boolean): boolean {
  return hasUnsavedChanges
}

export function useUnsavedChangesGuard({
  hasUnsavedChanges,
  onClose,
}: {
  hasUnsavedChanges: boolean
  onClose: () => void
}) {
  const [confirmOpen, setConfirmOpen] = useState(false)

  const requestClose = useCallback(() => {
    if (shouldConfirmDiscard(hasUnsavedChanges)) {
      setConfirmOpen(true)
      return
    }
    onClose()
  }, [hasUnsavedChanges, onClose])

  const discardAndClose = useCallback(() => {
    setConfirmOpen(false)
    onClose()
  }, [onClose])

  return {
    confirmOpen,
    discardAndClose,
    requestClose,
    setConfirmOpen,
  }
}
