import { useState, useRef, useEffect } from 'react'
import { Download, ChevronDown, FileCode, FileCode2, FileText, Terminal } from 'lucide-react'
import type { Job } from '@/api/types'
import {
  generateShellScript,
  generateMarkdown,
  generateJupyterBash,
  generateJupyterPython,
  downloadBlob,
} from '@/utils/export'
import { cn } from '@/utils/cn'

interface ExportMenuProps {
  jobs: Job[]
}

const exportOptions = [
  {
    id: 'shell',
    label: 'Shell Script',
    icon: Terminal,
    fn: generateShellScript,
    filename: 'aerolab-history.sh',
    mime: 'text/x-sh',
  },
  {
    id: 'markdown',
    label: 'Markdown',
    icon: FileText,
    fn: generateMarkdown,
    filename: 'aerolab-history.md',
    mime: 'text/markdown',
  },
  {
    id: 'jupyter-bash',
    label: 'Jupyter (Bash)',
    icon: FileCode,
    fn: generateJupyterBash,
    filename: 'aerolab-history-bash.ipynb',
    mime: 'application/json',
  },
  {
    id: 'jupyter-python',
    label: 'Jupyter (Python)',
    icon: FileCode2,
    fn: generateJupyterPython,
    filename: 'aerolab-history-python.ipynb',
    mime: 'application/json',
  },
] as const

export default function ExportMenu({ jobs }: ExportMenuProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const handleExport = (opt: (typeof exportOptions)[number]) => {
    const content = opt.fn(jobs)
    downloadBlob(content, opt.filename, opt.mime)
    setOpen(false)
  }

  const jobsWithCommands = jobs.filter((j) => j.cliCommand)
  const hasExportable = jobsWithCommands.length > 0

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        disabled={!hasExportable}
        className={cn(
          'flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium',
          'border border-slate-300 bg-white text-slate-700 hover:bg-slate-50',
          'dark:border-slate-600 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700',
          'disabled:opacity-50 disabled:cursor-not-allowed'
        )}
        title={hasExportable ? 'Export job history' : 'No jobs with commands to export'}
      >
        <Download className="h-4 w-4" />
        Export
        <ChevronDown className="h-4 w-4" />
      </button>
      {open && (
        <div
          className={cn(
            'absolute right-0 top-full z-50 mt-1 min-w-[200px] rounded-lg border shadow-lg',
            'border-slate-200 bg-white dark:border-slate-600 dark:bg-slate-800'
          )}
        >
          {exportOptions.map((opt) => {
            const Icon = opt.icon
            return (
              <button
                key={opt.id}
                type="button"
                onClick={() => handleExport(opt)}
                className={cn(
                  'flex w-full items-center gap-2 px-3 py-2 text-left text-sm',
                  'text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-700'
                )
              }
              >
                <Icon className="h-4 w-4 shrink-0" />
                {opt.label}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
