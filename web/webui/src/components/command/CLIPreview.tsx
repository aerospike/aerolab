import { useCLIPreview } from '@/hooks/useCLIPreview'
import { usePreferences } from '@/hooks/usePreferences'
import { highlightCLI } from '@/utils/cli'
import { cn } from '@/utils/cn'
import { toast } from 'sonner'

interface CLIPreviewProps {
  commandPath: string
  formValues: Record<string, unknown>
  enabled: boolean
  onRun: () => void
}

export default function CLIPreview({
  commandPath,
  formValues,
  enabled,
  onRun,
}: CLIPreviewProps) {
  const { shortSwitches, setShortSwitches, defaultSwitches, setDefaultSwitches, sidebarCollapsed } =
    usePreferences()
  const { cli, isLoading, refresh } = useCLIPreview(
    commandPath,
    formValues,
    enabled,
    shortSwitches,
    defaultSwitches
  )
  const tokens = highlightCLI(cli)
  const sidebarWidth = sidebarCollapsed ? '4rem' : '16rem'

  return (
    <div
      className="fixed bottom-0 right-0 z-50 border-t border-slate-700 bg-slate-800 px-4 py-3 text-slate-200 shadow-[0_-4px_6px_-1px_rgba(0,0,0,0.3)]"
      style={{ left: sidebarWidth }}
    >
      <div className="flex flex-wrap items-center gap-4">
        <label className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={shortSwitches}
            onChange={(e) => setShortSwitches(e.target.checked)}
            className="h-4 w-4 rounded border-slate-600 bg-slate-700 text-blue-500 focus:ring-blue-500"
          />
          <span className="text-sm">Short switches</span>
        </label>
        <label className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={defaultSwitches}
            onChange={(e) => setDefaultSwitches(e.target.checked)}
            className="h-4 w-4 rounded border-slate-600 bg-slate-700 text-blue-500 focus:ring-blue-500"
          />
          <span className="text-sm">Show defaults</span>
        </label>
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <code className="min-h-[2rem] flex-1 overflow-x-auto rounded bg-slate-900 px-3 py-2 font-mono text-sm">
          {isLoading ? (
            <span className="animate-pulse text-slate-500">Loading...</span>
          ) : tokens.length > 0 ? (
            tokens.map(({ token, className }, i) => (
              <span key={i}>
                {i > 0 && ' '}
                <span className={cn('cli-token', `cli-${className}`)}>{token}</span>
              </span>
            ))
          ) : (
            <span className="text-slate-500">No command</span>
          )}
        </code>

        <button
          type="button"
          onClick={() => {
            if (cli) {
              navigator.clipboard.writeText(cli)
              toast.success('Copied!')
            }
          }}
          className="rounded-md border border-slate-600 bg-slate-700 px-3 py-2 text-sm font-medium text-slate-200 hover:bg-slate-600"
        >
          Copy
        </button>
        <button
          type="button"
          onClick={refresh}
          disabled={isLoading || !enabled || !commandPath}
          className="rounded-md border border-slate-600 bg-slate-700 px-3 py-2 text-sm font-medium text-slate-200 hover:bg-slate-600 disabled:opacity-50"
        >
          Refresh
        </button>
        <button
          type="button"
          onClick={onRun}
          className="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700"
        >
          Run
        </button>
      </div>
    </div>
  )
}
