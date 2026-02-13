import { cn } from '@/utils/cn'

interface SpinnerProps {
  className?: string
  text?: string
}

export default function Spinner({ className, text }: SpinnerProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center gap-4', className)}>
      <div
        className="h-8 w-8 animate-spin rounded-full border-2 border-slate-300 border-t-blue-600 dark:border-slate-600 dark:border-t-blue-500"
        role="status"
        aria-label="Loading"
      />
      {text && (
        <p className="text-sm text-slate-600 dark:text-slate-400">{text}</p>
      )}
    </div>
  )
}
