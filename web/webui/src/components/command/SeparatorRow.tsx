interface SeparatorRowProps {
  label: string
}

export default function SeparatorRow({ label }: SeparatorRowProps) {
  return (
    <div className="flex items-center gap-4 py-2">
      <div className="h-px flex-1 bg-slate-200 dark:bg-slate-600" />
      <span className="rounded-full bg-indigo-100 px-3 py-1 text-xs font-medium text-indigo-800 dark:bg-indigo-900/50 dark:text-indigo-200">
        {label}
      </span>
      <div className="h-px flex-1 bg-slate-200 dark:bg-slate-600" />
    </div>
  )
}
