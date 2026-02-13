import { PromptDialog } from '@/components/common/PromptDialog'

interface ExpiryPromptProps {
  isOpen: boolean
  onClose: () => void
  onSubmit: (expiry: string) => void
}

export function ExpiryPrompt({ isOpen, onClose, onSubmit }: ExpiryPromptProps) {
  return (
    <PromptDialog
      isOpen={isOpen}
      onClose={onClose}
      onSubmit={onSubmit}
      title="Change Expiry"
      message="Enter new expiry duration"
      defaultValue="30h0m0s"
      placeholder="30h0m0s"
    />
  )
}
