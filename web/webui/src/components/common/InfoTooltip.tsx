interface InfoTooltipProps {
  text: string
}

export default function InfoTooltip({ text }: InfoTooltipProps) {
  return (
    <span className="group/tip relative ml-1 inline-flex cursor-help text-slate-400 hover:text-slate-600 dark:hover:text-slate-300">
      ℹ️
      <span className="pointer-events-none absolute left-full top-1/2 z-50 ml-2 hidden w-max max-w-xs -translate-y-1/2 rounded-md bg-slate-800 px-3 py-2 text-xs font-normal text-white shadow-lg group-hover/tip:block dark:bg-slate-700">
        {text}
        <span className="absolute right-full top-1/2 -translate-y-1/2 border-4 border-transparent border-r-slate-800 dark:border-r-slate-700" />
      </span>
    </span>
  )
}
